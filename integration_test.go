//go:build integration

package claudeagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// skipIfNoToken skips the test if no OAuth token is available.
func skipIfNoToken(t *testing.T) {
	t.Helper()
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" &&
		os.Getenv("ANTHROPIC_API_KEY") == "" {

		t.Skip("CLAUDE_CODE_OAUTH_TOKEN or ANTHROPIC_API_KEY " +
			"required for integration tests")
	}
}

// skipIfNoCLI skips the test if the Claude CLI is not installed.
func skipIfNoCLI(t *testing.T) {
	t.Helper()
	_, err := DiscoverCLIPath(&Options{})
	if err != nil {
		t.Skip("claude CLI not found in PATH")
	}
}

// isolatedClientOptions returns options that isolate the test from the
// local Claude Code configuration (user settings, hooks, skills, sessions).
// Creates a temporary config directory to completely sandbox the CLI.
func isolatedClientOptions(t *testing.T) []Option {
	t.Helper()

	// Create a temp directory for this test's config.
	// This prevents the CLI from loading ~/.claude settings/hooks.
	configDir := filepath.Join(t.TempDir(), ".claude")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create temp config dir: %v", err)
	}

	return []Option{
		// Use isolated config directory.
		WithConfigDir(configDir),
		// Don't save sessions to disk.
		WithNoSessionPersistence(),
		// Don't load user/project skills.
		WithSkillsDisabled(),
		// Don't load user/project settings.
		WithSettingSources(nil),
	}
}

// TestIntegrationBasicQuery tests a simple query-response flow with the real
// CLI.
func TestIntegrationBasicQuery(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Logf("Creating client...")
	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. Keep responses very brief.",
		),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()
	t.Logf("Client created")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var gotResponse bool
	var gotResult bool

	query := "Reply with exactly: Hello from integration test"
	t.Logf("Connecting...")
	err = client.Connect(ctx)
	require.NoError(t, err)
	t.Logf("Connected, checking if alive...")

	// Wait a moment for CLI to initialize
	time.Sleep(100 * time.Millisecond)

	// The subprocess should be running now

	t.Logf("Sending query: %s", query)

	// Test sending directly first
	msgCount := 0
	for msg := range client.Query(ctx, query) {
		msgCount++
		t.Logf("Received message type: %T", msg)
		switch m := msg.(type) {
		case AssistantMessage:
			gotResponse = true
			t.Logf("Assistant: %s", m.ContentText())
		case ResultMessage:
			gotResult = true
			t.Logf("Result: status=%s, cost=$%.4f",
				m.Status, m.TotalCostUSD)
		case SystemMessage:
			t.Logf("System: subtype=%s", m.Subtype)
		default:
			t.Logf("Other message: %+v", msg)
		}
	}
	t.Logf("Query loop finished")

	assert.True(t, gotResponse, "expected assistant response")
	assert.True(t, gotResult, "expected result message")
}

// TestIntegrationStreamConversation tests multi-turn streaming conversation.
func TestIntegrationStreamConversation(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. Keep responses to one sentence.",
		),
		WithMaxTurns(2),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	stream, err := client.Stream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	// First message.
	err = stream.Send(ctx, "Say hello")
	require.NoError(t, err)

	// Wait for first response.
	var firstResponse string
	for msg := range stream.Messages() {
		if m, ok := msg.(AssistantMessage); ok {
			firstResponse = m.ContentText()
			t.Logf("First response: %s", firstResponse)
		}
		if _, ok := msg.(ResultMessage); ok {
			break
		}
	}

	assert.NotEmpty(t, firstResponse, "expected first response")

	// Session ID should be set.
	sessionID := stream.SessionID()
	t.Logf("Session ID: %s", sessionID)
}

// TestIntegrationPermissionCallback tests permission callback invocation.
// Uses MCP server to ensure Claude must use a tool and trigger permission check.
func TestIntegrationPermissionCallback(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Build the example MCP server.
	mcpServerPath := filepath.Join(t.TempDir(), "example-mcp-server")
	buildCmd := exec.Command("go", "build", "-o", mcpServerPath, "./cmd/example-mcp-server")
	buildCmd.Dir = "."
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build MCP server: %v\n%s", err, out)
	}

	permissionCalled := false
	var requestedTool string

	canUseTool := func(ctx context.Context,
		req ToolPermissionRequest) PermissionResult {

		permissionCalled = true
		requestedTool = req.ToolName
		t.Logf("Permission requested for tool: %s", req.ToolName)
		t.Logf("Tool arguments: %s", string(req.Arguments))
		// Allow the tool.
		return PermissionAllow{}
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. When asked to add numbers, "+
				"you MUST use the add_numbers tool. Do not calculate manually.",
		),
		WithMCPServers(map[string]MCPServerConfig{
			"example": {
				Type:    "stdio",
				Command: mcpServerPath,
			},
		}),
		WithStrictMCPConfig(true),
		WithPermissionMode(PermissionModeDefault),
		WithCanUseTool(canUseTool),
		WithMaxTurns(5),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Ask Claude to use a tool, which should trigger permission callback.
	for msg := range client.Query(ctx, "Use the add_numbers tool to add 7 and 5.") {
		switch m := msg.(type) {
		case AssistantMessage:
			t.Logf("Response: %s", m.ContentText())
		case ResultMessage:
			t.Logf("Result: status=%s, cost=$%.4f", m.Status, m.TotalCostUSD)
		}
	}

	// Verify permission callback was invoked.
	assert.True(t, permissionCalled, "expected permission callback to be called")
	if permissionCalled {
		assert.Contains(t, requestedTool, "add_numbers",
			"expected permission request for add_numbers tool")
	}
}

func TestIntegrationPermissionDeniedMessage(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	mcpServerPath := filepath.Join(t.TempDir(), "example-mcp-server")
	buildCmd := exec.Command("go", "build", "-o", mcpServerPath, "./cmd/example-mcp-server")
	buildCmd.Dir = "."
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build MCP server: %v\n%s", err, out)
	}

	permissionCalled := false
	canUseTool := func(ctx context.Context,
		req ToolPermissionRequest) PermissionResult {

		permissionCalled = true
		t.Logf("Permission requested for tool: %s", req.ToolName)
		return PermissionDeny{Reason: "integration test denied tool use"}
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. When asked to add numbers, "+
				"you MUST use the add_numbers tool. Do not calculate manually.",
		),
		WithMCPServers(map[string]MCPServerConfig{
			"example": {
				Type:    "stdio",
				Command: mcpServerPath,
			},
		}),
		WithStrictMCPConfig(true),
		WithPermissionMode(PermissionModeDefault),
		WithCanUseTool(canUseTool),
		WithMaxTurns(5),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var deniedMessages []PermissionDeniedMessage
	for msg := range client.Query(ctx, "Use the add_numbers tool to add 7 and 5.") {
		switch m := msg.(type) {
		case PermissionDeniedMessage:
			deniedMessages = append(deniedMessages, m)
		case AssistantMessage:
			t.Logf("Response: %s", m.ContentText())
		case ResultMessage:
			t.Logf("Result: status=%s, cost=$%.4f", m.Status, m.TotalCostUSD)
		}
	}

	require.True(t, permissionCalled, "expected permission callback to be called")
	if len(deniedMessages) == 0 {
		t.Skip("permission_denied emission depends on CLI build supporting v0.3.150 SDKPermissionDeniedMessage")
	}

	var found bool
	for _, msg := range deniedMessages {
		if msg.ToolName != "add_numbers" {
			continue
		}
		found = true
		assert.NotEmpty(t, msg.ToolUseID)
		assert.NotEmpty(t, msg.Message)
		assert.NotEmpty(t, msg.UUID)
		assert.NotEmpty(t, msg.SessionID)
	}
	assert.True(t, found, "expected permission_denied message for add_numbers")
}

// TestIntegrationHooks tests hook callback invocation.
func TestIntegrationHooks(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	var hooksCalled []string

	promptSubmitHook := func(ctx context.Context,
		input HookInput) (HookResult, error) {

		hooksCalled = append(hooksCalled, "UserPromptSubmit")
		t.Logf("UserPromptSubmit hook called")
		return HookResult{Continue: true}, nil
	}

	stopHook := func(ctx context.Context,
		input HookInput) (HookResult, error) {

		hooksCalled = append(hooksCalled, "Stop")
		t.Logf("Stop hook called")
		return HookResult{Continue: true}, nil
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt("You are a helpful assistant."),
		WithHooks(map[HookType][]HookConfig{
			HookTypeUserPromptSubmit: {
				{Matcher: "*", Callback: promptSubmitHook},
			},
			HookTypeStop: {
				{Matcher: "*", Callback: stopHook},
			},
		}),
		WithMaxTurns(1),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for msg := range client.Query(ctx, "Say hello") {
		if m, ok := msg.(AssistantMessage); ok {
			t.Logf("Response: %s", m.ContentText())
		}
	}

	t.Logf("Hooks called: %v", hooksCalled)
}

// TestIntegrationModelSelection tests using a specific model.
func TestIntegrationModelSelection(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithModel("claude-sonnet-4-5-20250929"),
		WithSystemPrompt("You are a helpful assistant. Be very brief."),
		WithMaxTurns(1),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var gotResponse bool
	for msg := range client.Query(ctx, "What model are you?") {
		if m, ok := msg.(AssistantMessage); ok {
			gotResponse = true
			t.Logf("Response: %s", m.ContentText())
		}
	}

	assert.True(t, gotResponse)
}

// TestIntegrationUsageTracking tests that usage statistics are returned.
func TestIntegrationUsageTracking(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt("You are a helpful assistant."),
		WithMaxTurns(1),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var result *ResultMessage
	for msg := range client.Query(ctx, "Say hi") {
		if m, ok := msg.(ResultMessage); ok {
			result = &m
		}
	}

	require.NotNil(t, result, "expected result message")
	t.Logf("Total cost: $%.6f", result.TotalCostUSD)
	t.Logf("Duration: %dms", result.DurationMs)

	// Cost should be positive (we did make an API call).
	assert.Greater(t, result.TotalCostUSD, 0.0, "expected positive cost")
}

// TestIntegrationContextCancellation tests that context cancellation stops the
// query.
func TestIntegrationContextCancellation(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt("You are a helpful assistant."),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	// Very short timeout to trigger cancellation.
	ctx, cancel := context.WithTimeout(
		context.Background(), 100*time.Millisecond,
	)
	defer cancel()

	messageCount := 0
	query := "Write a very long essay about the history of computing."
	for range client.Query(ctx, query) {
		messageCount++
	}

	// Should have stopped early due to timeout.
	t.Logf("Received %d messages before timeout", messageCount)
}

// TestIntegrationSkills tests that skills are loaded.
// Note: This test intentionally loads user/project skills to verify loading.
func TestIntegrationSkills(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	client, err := NewClient(
		WithNoSessionPersistence(), // Still avoid polluting sessions
		WithSkills(SkillsConfig{
			EnableSkills:   true,
			SettingSources: []string{"user", "project"},
		}),
	)
	require.NoError(t, err)
	defer client.Close()

	skills := client.ListSkills()
	t.Logf("Loaded %d skills", len(skills))
	for _, s := range skills {
		t.Logf("  - %s: %s", s.Name, s.Description)
	}
}

// TestIntegrationMCPServer tests MCP server integration with Claude.
// This test builds and runs an example MCP server, then verifies Claude can
// call the tools provided by that server.
func TestIntegrationMCPServer(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Build the example MCP server.
	mcpServerPath := filepath.Join(t.TempDir(), "example-mcp-server")
	buildCmd := exec.Command(
		"go", "build",
		"-o", mcpServerPath,
		"./cmd/example-mcp-server",
	)
	buildCmd.Dir = "."
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build MCP server: %v\n%s", err, out)
	}
	t.Logf("Built MCP server: %s", mcpServerPath)

	// Create client with MCP server configured.
	// Use bypass permissions to allow the MCP tool to run.
	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. When asked to add numbers, "+
				"you MUST use the add_numbers tool from the example MCP server. "+
				"Do not calculate manually.",
		),
		WithMCPServers(map[string]MCPServerConfig{
			"example": {
				Type:    "stdio",
				Command: mcpServerPath,
			},
		}),
		// Only use our MCP config, ignore any system configs.
		WithStrictMCPConfig(true),
		// Bypass permissions for testing.
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(5),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Ask Claude to use the MCP tool.
	query := "Please use the add_numbers tool to add 5 and 3. " +
		"Report the exact result from the tool."

	t.Logf("Query: %s", query)

	var gotToolUse bool
	var gotResponse bool
	var responseText string

	for msg := range client.Query(ctx, query) {
		switch m := msg.(type) {
		case AssistantMessage:
			text := m.ContentText()
			if text != "" {
				responseText = text
				gotResponse = true
				t.Logf("Assistant: %s", text)
			}
			// Check for tool use in content blocks.
			for _, block := range m.Message.Content {
				if block.Type == "tool_use" {
					gotToolUse = true
					t.Logf("Tool use: %s", block.Name)
				}
			}
		case ResultMessage:
			t.Logf("Result: status=%s, cost=$%.4f", m.Status, m.TotalCostUSD)
		case SystemMessage:
			t.Logf("System: subtype=%s", m.Subtype)
		}
	}

	// Verify the tool was used and the result is correct.
	assert.True(t, gotResponse, "expected assistant response")

	// The response should contain "8" (5 + 3).
	assert.Contains(t, responseText, "8",
		"expected response to contain the sum 8")

	// Log whether tool was detected (may vary based on Claude's behavior).
	t.Logf("Tool use detected: %v", gotToolUse)
}

// TestIntegrationSDKMCPServer tests in-process SDK MCP tools.
//
// This test creates an in-process MCP server (no separate binary) and
// asks Claude to use a tool from it. Tool calls are routed through
// the control channel to the SDK.
func TestIntegrationSDKMCPServer(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Testing if CLI supports SDK MCP servers.

	// Define typed args.
	type AddArgs struct {
		A int `json:"a"`
		B int `json:"b"`
	}

	// Create an in-process MCP server with tools using the new API.
	server := CreateMcpServer(McpServerOptions{
		Name:    "calculator",
		Version: "1.0.0",
		Tools: []ToolRegistrar{
			ToolWithSchema("add_numbers", "Add two numbers together and return the sum",
				map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"a": map[string]interface{}{
							"type":        "integer",
							"description": "First number",
						},
						"b": map[string]interface{}{
							"type":        "integer",
							"description": "Second number",
						},
					},
					"required": []string{"a", "b"},
				},
				func(ctx context.Context, args AddArgs) (ToolResult, error) {
					sum := args.A + args.B
					return TextResult(fmt.Sprintf("%d", sum)), nil
				},
			),
		},
	})

	// Create client with in-process MCP server.
	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. When asked to add numbers, "+
				"you MUST use the add_numbers tool. Do not calculate manually.",
		),
		WithMcpServer("calculator", server),
		// Bypass permissions for testing.
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(5),
		// Log stderr to see CLI errors.
		WithStderr(func(data string) {
			t.Logf("CLI stderr: %s", data)
		}),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Ask Claude to use the MCP tool.
	query := "Please use the add_numbers tool to add 7 and 4. " +
		"Report the exact result from the tool."

	t.Logf("Query: %s", query)

	var gotResponse bool
	var responseText string

	for msg := range client.Query(ctx, query) {
		switch m := msg.(type) {
		case AssistantMessage:
			text := m.ContentText()
			if text != "" {
				responseText = text
				gotResponse = true
				t.Logf("Assistant: %s", text)
			}
			// Log tool uses to see if SDK MCP tools are called.
			for _, block := range m.Message.Content {
				if block.Type == "tool_use" {
					t.Logf("TOOL_USE: name=%s id=%s", block.Name, block.ID)
				}
			}
		case ResultMessage:
			t.Logf("Result: status=%s, cost=$%.4f", m.Status, m.TotalCostUSD)
		case SystemMessage:
			t.Logf("System: subtype=%s tools=%v mcp_servers=%v",
				m.Subtype, m.Tools, m.MCPServers)
		default:
			t.Logf("Other message type: %T", msg)
		}
	}

	// Verify the response contains the expected sum.
	assert.True(t, gotResponse, "expected assistant response")
	assert.Contains(t, responseText, "11",
		"expected response to contain the sum 11 (7 + 4)")
}

func TestIntegrationMCPTimeout(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("not directly triggerable from CLI without slow stdio MCP fixture: in-process SDK MCP servers are registered separately from MCPServerConfig timeout")
}

func TestIntegrationMCPAlwaysLoad(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI exposes prompt tool-loading state to SDK tests.
	t.Skip("not directly assertable from CLI: alwaysLoad affects prompt composition and tool-search gating, not anything observable on the SDK transport")
}

func TestIntegrationSDKMCPInstructions(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI exposes consumed MCP instructions to SDK tests.
	t.Skip("not directly assertable from CLI: MCP server instructions are consumed internally by the CLI for prompt composition")
}

func TestIntegrationHookCommandArgs(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill if the CLI exposes hook subprocess spawn details to SDK tests.
	t.Skip("not directly assertable from CLI: hook args is a settings shape consumed by the CLI's hook spawn path, not observable on the SDK transport")
}

func TestIntegrationHookContinueOnBlock(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill if the CLI exposes hook turn-continuation decisions to SDK tests.
	t.Skip("not directly assertable from CLI: continueOnBlock affects the CLI's turn-continuation decision after a blocking hook, not anything observable on the SDK transport")
}

func TestIntegrationHookTerminalSequence(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI test fixture can capture
	// terminal-escape bytes emitted by a hook return; currently those
	// go straight to the controlling TTY and aren't observable on the
	// SDK transport.
	t.Skip("not directly assertable from CLI: terminalSequence bytes are written to the controlling terminal, not surfaced on the SDK transport")
}

func TestIntegrationHookSuppressOriginalPrompt(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI test fixture can assert the
	// block-message body the CLI returns when a UserPromptSubmit hook
	// returns suppressOriginalPrompt: true; the omission is a CLI
	// rendering behavior not directly observable on the SDK transport.
	t.Skip("not directly assertable from CLI: suppressOriginalPrompt is a CLI block-message rendering behavior, not surfaced on the SDK transport")
}

func TestIntegrationTaskStartedBackgrounding(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Probed against CLI 2.1.222 with a prompt that spawns one Explore
	// subagent: the task_started frame carries task_id, tool_use_id,
	// description, subagent_type and task_type "local_agent", but neither
	// is_backgrounded nor spawn_depth. The runner CLI predates both fields,
	// so a live assertion could only re-assert the absent state.
	t.Skip("not triggerable from CLI: runner CLI 2.1.222 omits is_backgrounded and spawn_depth from task_started")
}

func TestIntegrationInitEffort(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// effort is published on Remote Control bridge init frames; a subprocess
	// transport is not one. Verified against CLI 2.1.222: the init message
	// carries apiKeySource and capabilities but no effort key at all, so a
	// live assertion here could only ever re-assert the absent state.
	t.Skip("not triggerable from CLI: effort is published on Remote Control bridge init frames, not on the subprocess transport's init message")
}

func TestIntegrationSessionStartReloadSkills(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("not triggerable from CLI: SessionStart hook reload-skills behavior requires a CLAUDE_SKILLS_DIR change between session start and first message; not exercisable in standard integration runs")
}

func TestIntegrationSessionTitleField(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("not triggerable from CLI: session_title only emits when user explicitly renames the session, which the standard integration run does not invoke")
}

func TestIntegrationMessageDisplayHook(t *testing.T) {
	// Registering HookTypeMessageDisplay against a streamed assistant turn
	// (with WithMaxTurns(1) and a normal prompt) yields zero callback
	// invocations end-to-end against the v0.3.168 CLI. The assistant message
	// arrives on the SDK transport but no MessageDisplay hook events do.
	// Tracked in memory/catchup-v0.3.168/INTEGRATION-FOLLOWUPS.md.
	t.Skip("not triggerable from CLI: v0.3.168 CLI does not emit MessageDisplay hook events to SDK transports during assistant streaming")
}

func TestIntegrationPostToolUseUpdatedToolOutput(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when a deterministic PostToolUse rewrite fixture is available.
	t.Skip("not triggerable from CLI without a deterministic tool call whose output a PostToolUse hook rewrites; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationStopHookBackgroundTasks(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when a deterministic long-running background task fixture is available.
	t.Skip("not triggerable from CLI without a long-running background task; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationStopHookSessionCrons(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when a deterministic session cron fixture is available.
	t.Skip("not triggerable from CLI without scheduling a session cron; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationStopHookAdditionalContext(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("not triggerable from CLI: hookSpecificOutput.additionalContext on Stop / SubagentStop requires the model to consume the context and continue the turn, which is not assertable from the SDK transport in standard integration runs; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationAssistantMessageSubagentFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("requires running a subagent task end-to-end; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationUserMessageSubagentFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("requires CLI to emit a user-side tool-result message from a Task-tool subagent (populates user message subagent_type/task_description); tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationAssistantMessageError(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("requires an upstream API failure (e.g. auth/quota) to deterministically populate AssistantMessage.Error; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationResultMessageOriginTTFT(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("requires CLI to emit origin/ttft_ms on result events (host-side multi-actor or measured TTFT); tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationResultMessageTimingFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("not triggerable from CLI: ttft_stream_ms / time_to_request_ms / time_to_request_from_spawn_ms / warm_spare_claimed are populated by CLI-internal spawn-pool timing instrumentation not exercisable from the standard integration run; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationTaskLifecycleFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("requires CLI to emit a Task-tool subagent run (subagent_type) and a paused-status task_updated event; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationThinkingTokensMessage(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	t.Skip("not triggerable from CLI: requires a thinking-capable model with redacted-thinking phase, which the standard integration run does not invoke")
}

func TestIntegrationBaseHookInputEffort(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	type AddArgs struct {
		A int `json:"a"`
		B int `json:"b"`
	}

	server := CreateMcpServer(McpServerOptions{
		Name:    "calculator",
		Version: "1.0.0",
		Tools: []ToolRegistrar{
			ToolWithSchema("add_numbers", "Add two numbers together and return the sum",
				map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"a": map[string]interface{}{
							"type":        "integer",
							"description": "First number",
						},
						"b": map[string]interface{}{
							"type":        "integer",
							"description": "Second number",
						},
					},
					"required": []string{"a", "b"},
				},
				func(ctx context.Context, args AddArgs) (ToolResult, error) {
					return TextResult(fmt.Sprintf("%d", args.A+args.B)), nil
				},
			),
		},
	})

	var capturedEffort *HookEffort
	preToolUseHook := func(ctx context.Context, input HookInput) (HookResult, error) {
		capturedEffort = input.Base().Effort
		return HookResult{Continue: true}, nil
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. When asked to add numbers, "+
				"you MUST use the add_numbers tool. Do not calculate manually.",
		),
		WithMcpServer("calculator", server),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithEffort(EffortHigh),
		WithHooks(map[HookType][]HookConfig{
			HookTypePreToolUse: {
				{Matcher: "*", Callback: preToolUseHook},
			},
		}),
		WithMaxTurns(5),
		WithStderr(func(data string) {
			t.Logf("CLI stderr: %s", data)
		}),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for msg := range client.Query(ctx, "Please use the add_numbers tool to add 7 and 4.") {
		switch m := msg.(type) {
		case AssistantMessage:
			t.Logf("Assistant: %s", m.ContentText())
		case ResultMessage:
			t.Logf("Result: status=%s, cost=$%.4f", m.Status, m.TotalCostUSD)
		}
	}

	if capturedEffort == nil {
		// TODO(PR-10): Backfill once the integration fixture uses a CLI/model
		// combination that emits effort on PreToolUse hook payloads.
		t.Skip("PreToolUse hook fired without BaseHookInput.effort; current CLI/model fixture does not expose effort on hook callbacks")
	}

	assert.Contains(t, []EffortLevel{
		EffortLow,
		EffortMedium,
		EffortHigh,
		EffortXHigh,
		EffortMax,
	}, capturedEffort.Level)
}

// TestIntegrationBaseHookInputPromptID checks that the CLI populates
// BaseHookInput.prompt_id on hook callbacks — the UUID correlating a user
// prompt with its subsequent events (sdk.d.ts v0.3.201).
func TestIntegrationBaseHookInputPromptID(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	var capturedPromptID string
	preToolUseHook := func(ctx context.Context, input HookInput) (HookResult, error) {
		if capturedPromptID == "" {
			capturedPromptID = input.Base().PromptID
		}
		return HookResult{Continue: true}, nil
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt("You are a helpful assistant. When asked to list a directory, use the Bash tool with 'ls'."),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithHooks(map[HookType][]HookConfig{
			HookTypePreToolUse: {
				{Matcher: "*", Callback: preToolUseHook},
			},
		}),
		WithMaxTurns(5),
		WithStderr(func(data string) { t.Logf("CLI stderr: %s", data) }),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for msg := range client.Query(ctx, "Run 'ls' in the current directory using the Bash tool.") {
		if m, ok := msg.(ResultMessage); ok {
			t.Logf("Result: status=%s", m.Status)
		}
	}

	if capturedPromptID == "" {
		// TODO: Backfill once the integration fixture uses a CLI build that
		// emits prompt_id on hook callbacks.
		t.Skip("PreToolUse hook fired without BaseHookInput.prompt_id; current CLI fixture does not expose it")
	}
	assert.NotEmpty(t, capturedPromptID)
}

// TestIntegrationStopHookBlock tests that Stop hooks can block session exit
// and reinject a new prompt using the Decision/Reason/SystemMessage fields.
//
// This is the foundation for the Ralph Wiggum pattern.
func TestIntegrationStopHookBlock(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Track how many times the Stop hook is called.
	stopCount := 0
	maxStops := 2

	stopHook := func(ctx context.Context,
		input HookInput) (HookResult, error) {

		stopCount++
		t.Logf("Stop hook called, count=%d", stopCount)

		if stopCount >= maxStops {
			// Allow exit after max iterations.
			t.Logf("Allowing exit after %d stops", stopCount)
			return HookResult{
				Continue: true,
				Decision: "approve",
			}, nil
		}

		// Block exit and reinject a new prompt.
		return HookResult{
			Continue:      false,
			Decision:      "block",
			Reason:        "Please say 'iteration " + fmt.Sprintf("%d", stopCount+1) + "'",
			SystemMessage: fmt.Sprintf("Stop hook test: iteration %d of %d", stopCount, maxStops),
		}, nil
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. Follow instructions exactly.",
		),
		WithHooks(map[HookType][]HookConfig{
			HookTypeStop: {
				{Matcher: "*", Callback: stopHook},
			},
		}),
		WithMaxTurns(5),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Start with initial prompt.
	var responses []string
	for msg := range client.Query(ctx, "Please say 'iteration 1'") {
		if m, ok := msg.(AssistantMessage); ok {
			text := m.ContentText()
			if text != "" {
				responses = append(responses, text)
				t.Logf("Response: %s", text)
			}
		}
	}

	// Verify multiple iterations happened.
	t.Logf("Stop count: %d, responses: %d", stopCount, len(responses))
	assert.GreaterOrEqual(t, stopCount, 1, "expected Stop hook to be called at least once")
}

// TestIntegrationRalphLoop tests the full Ralph Wiggum loop pattern.
//
// This test uses a simple task that Claude can complete quickly: counting
// from 1 to a target number and outputting a completion promise.
func TestIntegrationRalphLoop(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	loop := NewRalphLoop(RalphConfig{
		Task: "Count from 1 to 3, outputting each number on its own line. " +
			"When you have counted to 3, output your completion signal.",
		CompletionPromise: "COUNTING_DONE",
		MaxIterations:     5,
	})

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. Follow instructions precisely. "+
				"When you complete a task, output the completion signal "+
				"wrapped in promise tags: <promise>SIGNAL</promise>",
		),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(3),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	t.Logf("Starting Ralph loop with max %d iterations", loop.Config().MaxIterations)

	var lastIter *Iteration
	for iter := range loop.Run(ctx, opts...) {
		lastIter = iter

		t.Logf("Iteration %d complete", iter.Number)
		t.Logf("  Complete: %v", iter.Complete)
		t.Logf("  Cost: $%.4f (total: $%.4f)", iter.CostUSD, iter.TotalCostUSD)
		t.Logf("  Messages: %d", len(iter.Messages))

		if iter.Error != nil {
			t.Logf("  Error: %v", iter.Error)
			break
		}

		// Log assistant responses.
		for _, msg := range iter.Messages {
			if m, ok := msg.(AssistantMessage); ok {
				text := m.ContentText()
				if text != "" {
					t.Logf("  Assistant: %s", text)
				}
			}
		}

		if iter.Complete {
			t.Logf("Task completed!")
			break
		}
	}

	require.NotNil(t, lastIter, "expected at least one iteration")
	t.Logf("Final iteration: %d, complete: %v", lastIter.Number, lastIter.Complete)
	t.Logf("Total cost: $%.4f", loop.TotalCost())
}

// TestIntegrationRalphLoopWithMCP tests Ralph loop with MCP tools.
//
// This test combines the Ralph loop with an in-process MCP server to verify
// that iterative tool-based workflows work correctly.
func TestIntegrationRalphLoopWithMCP(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Define typed args for our counter tool.
	type IncrementArgs struct {
		Current int `json:"current"`
	}

	// Track calls to the tool.
	toolCalls := 0

	// Create an in-process MCP server with a simple counter tool.
	server := CreateMcpServer(McpServerOptions{
		Name:    "counter",
		Version: "1.0.0",
		Tools: []ToolRegistrar{
			ToolWithSchema("increment", "Increment a number by 1",
				map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"current": map[string]interface{}{
							"type":        "integer",
							"description": "Current number to increment",
						},
					},
					"required": []string{"current"},
				},
				func(ctx context.Context, args IncrementArgs) (ToolResult, error) {
					toolCalls++
					next := args.Current + 1
					return TextResult(fmt.Sprintf("Result: %d", next)), nil
				},
			),
		},
	})

	loop := NewRalphLoop(RalphConfig{
		Task: "Use the increment tool to count from 0 to 2. " +
			"Call increment(0), then increment(1), then increment(2). " +
			"After you get the result 3, output your completion signal.",
		CompletionPromise: "INCREMENTED",
		MaxIterations:     5,
	})

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant. Use the increment tool as instructed. "+
				"When complete, output: <promise>INCREMENTED</promise>",
		),
		WithMcpServer("counter", server),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(10), // Allow multiple tool calls per iteration.
	)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	t.Logf("Starting Ralph loop with MCP tools")

	var lastIter *Iteration
	for iter := range loop.Run(ctx, opts...) {
		lastIter = iter

		t.Logf("Iteration %d: complete=%v, messages=%d, tool_calls=%d",
			iter.Number, iter.Complete, len(iter.Messages), toolCalls)

		if iter.Error != nil {
			t.Logf("Error: %v", iter.Error)
			break
		}

		if iter.Complete {
			t.Logf("Task completed!")
			break
		}
	}

	require.NotNil(t, lastIter, "expected at least one iteration")
	t.Logf("Tool calls made: %d", toolCalls)
	t.Logf("Total cost: $%.4f", loop.TotalCost())

	// Verify the increment tool was called at least once.
	assert.GreaterOrEqual(t, toolCalls, 1,
		"expected increment tool to be called at least once")
}

// TestIntegrationAskUserQuestionCallback tests the callback-based question
// handling. This uses a system prompt that encourages Claude to ask questions.
func TestIntegrationAskUserQuestionCallback(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	var questionReceived bool
	var questionText string

	// Create a handler that auto-answers questions.
	handler := func(ctx context.Context, qs QuestionSet) (Answers, error) {
		questionReceived = true
		if len(qs.Questions) > 0 {
			questionText = qs.Questions[0].Question
		}
		t.Logf("Question received: %+v", qs)

		// Auto-answer with first option if available, else "yes".
		answers := make(Answers)
		for i, q := range qs.Questions {
			if len(q.Options) > 0 {
				answers[fmt.Sprintf("q_%d", i)] = q.Options[0].Label
			} else {
				answers[fmt.Sprintf("q_%d", i)] = "yes"
			}
		}
		return answers, nil
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant that ALWAYS asks clarifying questions "+
				"before proceeding with any task. Use the AskUserQuestion tool to "+
				"ask the user at least one question before giving your final answer. "+
				"This is required for every interaction.",
		),
		WithAskUserQuestionHandler(handler),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(5),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	query := "I want to create a new project."
	t.Logf("Query: %s", query)

	var gotResponse bool

	for msg := range client.Query(ctx, query) {
		switch m := msg.(type) {
		case AssistantMessage:
			text := m.ContentText()
			if text != "" {
				gotResponse = true
				t.Logf("Assistant: %s", text)
			}
		case ResultMessage:
			t.Logf("Result: status=%s", m.Status)
		}
	}

	// Log whether question was asked (may vary based on Claude's behavior).
	t.Logf("Question received via callback: %v", questionReceived)
	if questionReceived {
		t.Logf("Question text: %s", questionText)
	}

	assert.True(t, gotResponse, "expected assistant response")
}

// TestIntegrationAskUserQuestionCallbackError tests that errors from the
// callback handler are properly sent back to Claude so the conversation
// doesn't hang.
func TestIntegrationAskUserQuestionCallbackError(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	errorReturned := false

	// Create a handler that returns an error.
	handler := func(ctx context.Context, qs QuestionSet) (Answers, error) {
		errorReturned = true
		t.Logf("Question received, returning error")
		return nil, fmt.Errorf("simulated handler error")
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant that ALWAYS asks clarifying questions "+
				"before proceeding with any task. Use the AskUserQuestion tool to "+
				"ask the user at least one question before giving your final answer. "+
				"This is required for every interaction.",
		),
		WithAskUserQuestionHandler(handler),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(5),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	query := "I want to set up a new project."
	t.Logf("Query: %s", query)

	var gotResponse bool

	// Even with a handler error, the conversation should continue
	// (Claude receives the error and can respond appropriately).
	for msg := range client.Query(ctx, query) {
		switch m := msg.(type) {
		case AssistantMessage:
			text := m.ContentText()
			if text != "" {
				gotResponse = true
				t.Logf("Assistant: %s", text)
			}
		case ResultMessage:
			t.Logf("Result: status=%s", m.Status)
		}
	}

	t.Logf("Error returned from callback: %v", errorReturned)
	// The conversation should complete (not hang).
	assert.True(t, gotResponse, "expected assistant response after handler error")
}

// TestIntegrationQuestionMessage tests the QuestionMessage flow in Query().
// When no callback handler is configured, QuestionMessage should be yielded.
func TestIntegrationQuestionMessage(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You are a helpful assistant that ALWAYS asks clarifying questions "+
				"before proceeding with any task. Use the AskUserQuestion tool to "+
				"ask the user at least one question before giving your final answer. "+
				"This is required for every interaction.",
		),
		// No callback handler - QuestionMessage should be yielded.
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(5),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	query := "I want to set up a new database for my application."
	t.Logf("Query: %s", query)

	var gotQuestionMessage bool
	var gotResponse bool

	for msg := range client.Query(ctx, query) {
		switch m := msg.(type) {
		case QuestionMessage:
			gotQuestionMessage = true
			t.Logf("QuestionMessage received: %d questions", len(m.Questions))
			for i, q := range m.Questions {
				t.Logf("  Q%d: %s", i, q.Question)
				for j, opt := range q.Options {
					t.Logf("    Option %d: %s - %s", j, opt.Label, opt.Description)
				}
			}

			// Answer using the fluent API.
			var err error
			if len(m.Questions[0].Options) > 0 {
				err = m.Respond(m.AnswerAll(m.Q(0).SelectIndex(0)))
			} else {
				err = m.Respond(m.Answer(0, "PostgreSQL"))
			}
			if err != nil {
				t.Logf("Error responding: %v", err)
			}

		case AssistantMessage:
			text := m.ContentText()
			if text != "" {
				gotResponse = true
				t.Logf("Assistant: %s", text)
			}
		case ResultMessage:
			t.Logf("Result: status=%s", m.Status)
		}
	}

	// Log whether QuestionMessage was received (may vary based on Claude's behavior).
	t.Logf("QuestionMessage received: %v", gotQuestionMessage)
	assert.True(t, gotResponse, "expected assistant response")
}

// TestIntegrationSubagentQuestionAwareness tests that questions from subagents
// are correctly identified via IsFromSubagent().
//
// This test verifies:
// 1. QuestionSet.ParentToolUseID is populated when a question comes from a subagent
// 2. IsFromSubagent() returns true for subagent questions
//
// Note: Getting Claude to reliably invoke a subagent that asks questions is
// difficult to control in tests. The core logic is verified in ask_user_test.go.
// This test documents the expected behavior with custom agents.
func TestIntegrationSubagentQuestionAwareness(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	var questionsReceived []QuestionSet

	handler := func(ctx context.Context, qs QuestionSet) (Answers, error) {
		questionsReceived = append(questionsReceived, qs)
		t.Logf("Question received - IsFromSubagent: %v, ParentToolUseID: %v",
			qs.IsFromSubagent(), qs.ParentToolUseID)

		// Auto-answer with first option.
		answers := make(Answers)
		for i, q := range qs.Questions {
			if len(q.Options) > 0 {
				answers[fmt.Sprintf("q_%d", i)] = q.Options[0].Label
			} else {
				answers[fmt.Sprintf("q_%d", i)] = "yes"
			}
		}
		return answers, nil
	}

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt(
			"You have access to a research agent. When the user asks about "+
				"something complex, delegate to the research agent using the Task tool.",
		),
		WithAgents(map[string]AgentDefinition{
			"research": {
				Name:        "research",
				Description: "Research specialist that asks clarifying questions",
				Prompt: "You are a research assistant. ALWAYS ask the user a clarifying " +
					"question using the AskUserQuestion tool before providing any analysis.",
				Tools: []string{"AskUserQuestion", "WebSearch"},
			},
		}),
		WithAskUserQuestionHandler(handler),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(10),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Ask something that might trigger the research agent.
	query := "Research the latest developments in quantum computing for me."
	t.Logf("Query: %s", query)

	var gotResponse bool

	for msg := range client.Query(ctx, query) {
		switch m := msg.(type) {
		case AssistantMessage:
			text := m.ContentText()
			if text != "" {
				gotResponse = true
				// Check if this is from a subagent
				if m.ParentToolUseID != nil {
					t.Logf("Subagent message: %s", truncateString(text, 100))
				} else {
					t.Logf("Main agent: %s", truncateString(text, 100))
				}
			}
		case SubagentResultMessage:
			t.Logf("Subagent %s completed: %s", m.AgentName, m.Status)
		case ResultMessage:
			t.Logf("Result: status=%s, cost=$%.4f", m.Status, m.TotalCostUSD)
		}
	}

	// Log results.
	t.Logf("Total questions received: %d", len(questionsReceived))
	for i, qs := range questionsReceived {
		t.Logf("  Question %d: IsFromSubagent=%v", i, qs.IsFromSubagent())
	}

	assert.True(t, gotResponse, "expected assistant response")

	// Note: We can't guarantee a subagent question was asked, but if one was,
	// verify IsFromSubagent works correctly.
	for _, qs := range questionsReceived {
		if qs.ParentToolUseID != nil {
			t.Logf("SUCCESS: Received question from subagent with ParentToolUseID=%s",
				*qs.ParentToolUseID)
			assert.True(t, qs.IsFromSubagent(),
				"IsFromSubagent should return true when ParentToolUseID is set")
		}
	}
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// TestIntegrationWorkingDirectory tests that the subprocess runs in the
// specified working directory. This verifies that WithCwd actually affects
// Claude's working directory.
func TestIntegrationWorkingDirectory(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// Create a temp directory with a marker file.
	tempDir := t.TempDir()
	markerFile := filepath.Join(tempDir, "cwd_test_marker.txt")
	markerContent := "MARKER_CONTENT_12345"
	err := os.WriteFile(markerFile, []byte(markerContent), 0644)
	require.NoError(t, err, "failed to create marker file")

	// Create client with Cwd set to the temp directory.
	opts := append(isolatedClientOptions(t),
		WithCwd(tempDir),
		WithSystemPrompt(
			"You are a helpful assistant. When asked to read a file, use the "+
				"Read tool to read it. Report the exact contents.",
		),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(3),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Ask Claude to read the marker file using a relative path.
	// If cwd is set correctly, Claude should be able to find it.
	query := "Please read the file 'cwd_test_marker.txt' and tell me its contents exactly."
	t.Logf("Query: %s", query)
	t.Logf("Expected marker: %s", markerContent)

	var gotResponse bool
	var responseText string
	var usedReadTool bool

	for msg := range client.Query(ctx, query) {
		switch m := msg.(type) {
		case AssistantMessage:
			// Check for Read tool usage in content blocks.
			for _, block := range m.Message.Content {
				if block.Type == "tool_use" && block.Name == "Read" {
					usedReadTool = true
					t.Logf("Read tool invoked with input: %s", string(block.Input))
				}
			}
			text := m.ContentText()
			if text != "" {
				gotResponse = true
				responseText += text
				t.Logf("Assistant: %s", text)
			}
		case ResultMessage:
			t.Logf("Result: status=%s, cost=$%.4f", m.Status, m.TotalCostUSD)
		}
	}

	assert.True(t, gotResponse, "expected assistant response")
	assert.True(t, usedReadTool, "expected Read tool to be used for file access")
	assert.Contains(t, responseText, markerContent,
		"expected response to contain the marker content, indicating cwd was set correctly")
}

// TestIntegrationHookLifecycleMessages exercises the wire-protocol hook
// lifecycle messages (hook_started / hook_progress / hook_response) that are
// emitted by the CLI when a settings.json subprocess hook fires.
//
// These wire messages are distinct from the SDK's callback hooks (see
// TestIntegrationHooks): callback hooks are control-channel mediated and never
// surface as system subtype messages. The lifecycle messages we parse here
// only appear when the CLI runs an external command hook configured via a
// settings file.
func TestIntegrationHookLifecycleMessages(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	tmp := t.TempDir()
	settingsPath := filepath.Join(tmp, "settings.json")
	settings := Settings{
		Hooks: map[string][]SettingsHookMatcher{
			"UserPromptSubmit": {
				{
					Hooks: []SettingsHook{
						{
							Type:    "command",
							Command: `printf '{"continue":true}\n'`,
						},
					},
				},
			},
		},
	}
	settingsJSON, err := json.MarshalIndent(settings, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(settingsPath, settingsJSON, 0o644))

	opts := append(isolatedClientOptions(t),
		WithSettingsPath(settingsPath),
		WithExtraArgs(map[string]*string{
			"include-hook-events": nil,
		}),
		WithSystemPrompt("You are a helpful assistant. Keep responses very brief."),
		WithMaxTurns(1),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	require.NoError(t, client.Connect(ctx))

	startedByID := make(map[string]HookStartedMessage)
	var responses []HookResponseMessage
	var sawProgress bool
	var result *ResultMessage

	for msg := range client.Query(ctx, "Reply with exactly: ok") {
		switch m := msg.(type) {
		case HookStartedMessage:
			t.Logf("Hook started: id=%s name=%s event=%s",
				m.HookID, m.HookName, m.HookEvent)
			if m.HookEvent == "UserPromptSubmit" {
				startedByID[m.HookID] = m
			}
		case HookProgressMessage:
			if m.HookEvent == "UserPromptSubmit" {
				sawProgress = true
				t.Logf("Hook progress: id=%s output=%q",
					m.HookID, m.Output)
			}
		case HookResponseMessage:
			t.Logf("Hook response: id=%s name=%s event=%s outcome=%s output=%q",
				m.HookID, m.HookName, m.HookEvent, m.Outcome, m.Output)
			if m.HookEvent == "UserPromptSubmit" {
				responses = append(responses, m)
			}
		case ResultMessage:
			result = &m
		}
	}

	require.NotNil(t, result, "expected query to complete")
	assert.NotEmpty(t, startedByID, "expected hook_started for UserPromptSubmit")
	require.NotEmpty(t, responses, "expected hook_response for UserPromptSubmit")

	var matched bool
	for _, response := range responses {
		started, ok := startedByID[response.HookID]
		if !ok {
			continue
		}
		matched = true
		assert.NotEmpty(t, response.HookID)
		assert.Equal(t, started.HookName, response.HookName)
		assert.Equal(t, started.HookEvent, response.HookEvent)
		assert.NotEmpty(t, response.Outcome)
	}
	assert.True(t, matched, "expected hook_response hook_id to match hook_started")
	t.Logf("Observed hook_progress: %t", sawProgress)
}

// TestIntegrationStreamIntrospection exercises the cached-init readers added
// in PR 18 against the live CLI. The CLI's initialize response populates
// commands and models; we assert both are non-empty and that AccountInfo
// returns without error.
func TestIntegrationStreamIntrospection(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt("You are a helpful assistant."),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := client.Stream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	init, err := stream.InitializationResult()
	require.NoError(t, err)
	require.NotNil(t, init)

	commands, err := stream.SupportedCommands(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, commands, "expected at least one slash command from CLI")

	models, err := stream.SupportedModels(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, models, "expected at least one model from CLI")

	// Account may be empty under OAuth-token-only auth; just ensure no error.
	acct, err := stream.AccountInfo(ctx)
	require.NoError(t, err)
	if acct.APIProvider != "" {
		assert.Contains(t, []APIProvider{
			APIProviderFirstParty,
			APIProviderBedrock,
			APIProviderVertex,
			APIProviderFoundry,
			APIProviderAnthropicAWS,
			APIProviderMantle,
			APIProviderGateway,
		}, acct.APIProvider)
	}
}

// TestIntegrationStreamFileAndRuntime exercises PR 20 stream file/runtime
// control methods against the live CLI.
func TestIntegrationStreamFileAndRuntime(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	const markerContent = "hello-stream-readfile-#57"

	newFileStream := func(t *testing.T) (*Stream, string) {
		t.Helper()

		tempDir := t.TempDir()
		markerPath := filepath.Join(tempDir, "marker.txt")
		require.NoError(t, os.WriteFile(markerPath, []byte(markerContent), 0o644))

		opts := append(isolatedClientOptions(t),
			WithCwd(tempDir),
			WithSystemPrompt("You are a helpful assistant."),
			WithPermissionMode(PermissionModeBypassAll),
			WithAllowDangerouslySkipPermissions(true),
		)
		client, err := NewClient(opts...)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		t.Cleanup(cancel)

		stream, err := client.Stream(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, stream.Close())
		})

		return stream, markerPath
	}

	t.Run("read_file", func(t *testing.T) {
		stream, markerPath := newFileStream(t)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		got, err := stream.ReadFile(ctx, "marker.txt", &ReadFileOptions{})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, markerContent, got.Contents)
		assert.Equal(t, filepath.Clean(markerPath), filepath.Clean(got.AbsPath))
		assert.False(t, got.Truncated)
	})

	t.Run("read_file_base64", func(t *testing.T) {
		tempDir := t.TempDir()
		binaryPath := filepath.Join(tempDir, "marker.bin")
		binaryBytes := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 'h', 'i'}
		require.NoError(t, os.WriteFile(binaryPath, binaryBytes, 0o644))

		opts := append(isolatedClientOptions(t),
			WithCwd(tempDir),
			WithSystemPrompt("You are a helpful assistant."),
			WithPermissionMode(PermissionModeBypassAll),
			WithAllowDangerouslySkipPermissions(true),
		)
		client, err := NewClient(opts...)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, client.Close())
		})

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		stream, err := client.Stream(ctx)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, stream.Close())
		})

		got, err := stream.ReadFile(ctx, "marker.bin", &ReadFileOptions{
			Encoding: "base64",
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "base64", got.Encoding,
			"CLI should echo encoding=base64 when honored")

		decoded, err := base64.StdEncoding.DecodeString(got.Contents)
		require.NoError(t, err)
		assert.Equal(t, binaryBytes, decoded)
		assert.Equal(t, filepath.Clean(binaryPath), filepath.Clean(got.AbsPath))
	})

	t.Run("seed_read_state", func(t *testing.T) {
		stream, markerPath := newFileStream(t)
		info, err := os.Stat(markerPath)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		require.NoError(t, stream.SeedReadState(ctx, markerPath, info.ModTime().UnixMilli()))
	})

	t.Run("apply_flag_settings", func(t *testing.T) {
		stream, _ := newFileStream(t)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Empty-map call: baseline (was the original assertion).
		require.NoError(t, stream.ApplyFlagSettings(ctx, map[string]interface{}{}))

		// Set a flag key, then clear it with an explicit nil value. The CLI is
		// the authority on flag-key validation; there is no read-back API yet.
		require.NoError(t, stream.ApplyFlagSettings(ctx, map[string]interface{}{
			"verbose": true,
		}))
		require.NoError(t, stream.ApplyFlagSettings(ctx, map[string]interface{}{
			"verbose": nil,
		}))
	})

	t.Run("submit_feedback", func(t *testing.T) {
		stream, _ := newFileStream(t)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		require.NoError(t, stream.SubmitFeedback(ctx, "integration test ping"))
		require.NoError(t, stream.SubmitFeedback(ctx,
			"with surface", SubmitFeedbackOptions{Surface: "integration"}))
	})

	t.Run("deferred_runtime_methods", func(t *testing.T) {
		t.Skip("not triggerable from CLI yet: RewindFiles needs file " +
			"checkpointing setup, ReloadPlugins needs a plugin fixture, " +
			"and StopTask needs a running task identifier")
	})
}

func TestIntegrationBackgroundTasks(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	tempDir := t.TempDir()
	opts := append(isolatedClientOptions(t),
		WithCwd(tempDir),
		WithSystemPrompt(
			"You are a helpful assistant. When asked to run a shell command, "+
				"use Bash immediately and do not describe it first.",
		),
		WithPermissionMode(PermissionModeBypassAll),
		WithAllowDangerouslySkipPermissions(true),
		WithMaxTurns(3),
		WithStderr(func(data string) {
			t.Logf("CLI stderr: %s", data)
		}),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	stream, err := client.Stream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	require.NoError(t, stream.Send(ctx,
		"Run this exact Bash command and wait for it: sleep 30",
	))

	var (
		backgrounded   bool
		backgroundErr  error
		backgroundSent bool
		gotToolUse     bool
		toolUseID      string
		gotResult      bool
		gotInProgress  bool
	)

	for msg := range stream.Messages() {
		switch m := msg.(type) {
		case AssistantMessage:
			for _, block := range m.Message.Content {
				if block.Type != "tool_use" {
					continue
				}
				gotToolUse = true
				toolUseID = block.ID
				t.Logf("Tool use: name=%s id=%s", block.Name, block.ID)
				if !backgroundSent {
					backgroundSent = true
					backgrounded, backgroundErr = stream.BackgroundTasks(ctx, "")
					if backgroundErr != nil {
						if strings.Contains(backgroundErr.Error(), "background_tasks") {
							t.Skip("background_tasks control subtype depends on CLI build supporting v0.3.150")
						}
						require.NoError(t, backgroundErr)
					}
					assert.True(t, backgrounded)
				}
			}
		case UserMessage:
			if m.ParentToolUseID != nil && strings.Contains(
				strings.ToLower(fmt.Sprint(m.ToolUseResult)),
				"background",
			) {
				gotInProgress = true
			}
		case ResultMessage:
			gotResult = true
			t.Logf("Result: subtype=%s status=%s", m.Subtype, m.Status)
		case TaskNotificationMessage:
			t.Logf("Task notification: task_id=%s tool_use_id=%s status=%s",
				m.TaskID, m.ToolUseID, m.Status)
		}
	}

	require.True(t, gotToolUse, "expected a Bash tool_use")
	require.NotEmpty(t, toolUseID, "expected tool_use id")
	require.True(t, backgroundSent, "expected BackgroundTasks call")
	require.NoError(t, backgroundErr)
	assert.True(t, backgrounded)
	assert.True(t, gotInProgress, "expected background tool_result")
	assert.True(t, gotResult, "expected final result")
}

// TestIntegrationSettingsOptions is a slot for the PR 22 settings option
// surface. The transport unit tests assert exact --settings and
// --managed-settings argv emission; a live assertion needs a CLI-supported
// observable effect that is isolated from user/project settings and does not
// depend on local account/model policy. Once the harness exposes CLI argv
// capture or a stable settings-driven wire effect, replace this skip with an
// end-to-end assertion.
func TestIntegrationSettingsOptions(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	realCLI, err := DiscoverCLIPath(&Options{})
	require.NoError(t, err)

	runWithArgvCapture := func(t *testing.T, opts ...Option) []string {
		t.Helper()

		tmp := t.TempDir()
		argvLog := filepath.Join(tmp, "argv.json")
		shimPath := filepath.Join(tmp, "claude-shim.sh")
		shimBody := fmt.Sprintf(`#!/usr/bin/env bash
argv_log=%q
real_cli=%q
python3 - "$argv_log" "$@" <<'PY'
import json
import sys

with open(sys.argv[1], "w", encoding="utf-8") as f:
    json.dump(sys.argv[2:], f)
PY
exec "$real_cli" "$@"
`, argvLog, realCLI)
		require.NoError(t, os.WriteFile(shimPath, []byte(shimBody), 0o755))

		clientOpts := append(isolatedClientOptions(t), WithCLIPath(shimPath))
		clientOpts = append(clientOpts, opts...)

		client, err := NewClient(clientOpts...)
		require.NoError(t, err)
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		stream, err := client.Stream(ctx)
		require.NoError(t, err)
		defer stream.Close()

		init, err := stream.InitializationResult()
		require.NoError(t, err)
		require.NotNil(t, init)

		data, err := os.ReadFile(argvLog)
		require.NoError(t, err)

		var argv []string
		require.NoError(t, json.Unmarshal(data, &argv))
		return argv
	}

	argValue := func(t *testing.T, argv []string, flag string) string {
		t.Helper()
		for i, arg := range argv {
			if arg == flag {
				require.Less(t, i+1, len(argv), "expected value after %s in %v", flag, argv)
				return argv[i+1]
			}
		}
		require.Failf(t, "missing flag", "expected %s in %v", flag, argv)
		return ""
	}

	t.Run("settings_path", func(t *testing.T) {
		tmp := t.TempDir()
		settingsPath := filepath.Join(tmp, "user-settings.json")
		require.NoError(t, os.WriteFile(settingsPath, []byte("{}"), 0o644))

		argv := runWithArgvCapture(t, WithSettingsPath(settingsPath))
		assert.Equal(t, settingsPath, argValue(t, argv, "--settings"))
	})

	t.Run("inline_settings", func(t *testing.T) {
		want := Settings{
			Env: map[string]string{
				"CLAUDE_AGENT_SDK_GO_INTEGRATION": "settings",
			},
		}

		argv := runWithArgvCapture(t, WithSettings(want))
		var got Settings
		require.NoError(t, json.Unmarshal([]byte(argValue(t, argv, "--settings")), &got))
		assert.Equal(t, want.Env, got.Env)
	})

	t.Run("managed_settings", func(t *testing.T) {
		want := Settings{
			Env: map[string]string{
				"CLAUDE_AGENT_SDK_GO_INTEGRATION": "managed-settings",
			},
		}

		argv := runWithArgvCapture(t, WithManagedSettings(want))
		var got Settings
		require.NoError(t, json.Unmarshal([]byte(argValue(t, argv, "--managed-settings")), &got))
		assert.Equal(t, want.Env, got.Env)
	})

	// Only the gate is asserted live. Feeding a command-sourced marketplace to
	// a CLI that predates the variant makes it fail schema validation on the
	// managed tier and stall the handshake, so that half stays in
	// TestIntegrationSettingsMiscFields with the other source variants until
	// the installed binary is new enough to accept it.
	t.Run("disable_command_plugin_sources", func(t *testing.T) {
		disable := true

		argv := runWithArgvCapture(t, WithManagedSettings(Settings{
			DisableCommandPluginSources: &disable,
		}))

		var got Settings
		require.NoError(t, json.Unmarshal([]byte(argValue(t, argv, "--managed-settings")), &got))
		require.NotNil(t, got.DisableCommandPluginSources)
		assert.True(t, *got.DisableCommandPluginSources)
	})

	// Whether the CLI resolves these to their canonical spellings is its
	// business, and 2.1.222 predates them so it won't. What's ours is that they
	// serialize under their own keys, don't get folded into the canonical ones,
	// and don't trip managed-tier schema validation on the way through.
	t.Run("marketplace_aliases", func(t *testing.T) {
		want := Settings{
			AdditionalMarketplaces: map[string]SettingsMarketplace{
				"vendor": {
					Source: SettingsMarketplaceSource{
						"source":  string(SettingsMarketplaceSourceNPM),
						"package": "@vendor/plugins",
					},
				},
			},
			AllowedMarketplaces: []SettingsMarketplaceSource{
				{
					"source":      string(SettingsMarketplaceSourcePathPattern),
					"pathPattern": "^/opt/approved/",
				},
			},
		}

		argv := runWithArgvCapture(t, WithManagedSettings(want))
		var got Settings
		require.NoError(t, json.Unmarshal([]byte(argValue(t, argv, "--managed-settings")), &got))

		assert.Contains(t, got.AdditionalMarketplaces, "vendor")
		assert.Empty(t, got.ExtraKnownMarketplaces,
			"alias must not be folded into the canonical key")

		require.Len(t, got.AllowedMarketplaces, 1)
		assert.Equal(t, "^/opt/approved/", got.AllowedMarketplaces[0]["pathPattern"])
		assert.Empty(t, got.StrictKnownMarketplaces,
			"alias must not be folded into the canonical key")
	})

	// The login flow itself can't be driven here (the session is already
	// authenticated), so this covers the reachable half: the pair survives the
	// managed tier and the CLI starts on it.
	t.Run("force_login_gateway_url", func(t *testing.T) {
		want := Settings{
			ForceLoginMethod:     "gateway",
			ForceLoginGatewayURL: "https://gateway.corp.example",
		}

		argv := runWithArgvCapture(t, WithManagedSettings(want))
		var got Settings
		require.NoError(t, json.Unmarshal([]byte(argValue(t, argv, "--managed-settings")), &got))

		assert.Equal(t, "gateway", got.ForceLoginMethod)
		assert.Equal(t, want.ForceLoginGatewayURL, got.ForceLoginGatewayURL)
	})
}

func TestIntegrationSettingsManagedOrgFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI test fixture can deterministically inject a
	// managed-settings.json / MDM policy tier so the SDK can observe these
	// admin-controlled settings, including fallbackModel, through the
	// resolved-settings transport.
	t.Skip("not directly assertable from CLI: managed-org settings including fallbackModel are honored only from admin-controlled policy sources, not surfaced on user CLI flag entry points")
}

func TestIntegrationSettingsWorkflowsFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI test fixture can deterministically inject
	// workflows + ultracode + pluginSuggestionMarketplaces through managed or
	// session settings so the SDK can observe resolved workflow policy state.
	// Tracked in memory/catchup-v0.3.168/INTEGRATION-FOLLOWUPS.md.
	t.Skip("not triggerable from CLI: workflows + ultracode + pluginSuggestionMarketplaces are admin-policy-tier toggles not surfaced via standard CLI flags")
}

func TestIntegrationSettingsSandboxFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI test fixture can deterministically inject
	// sandbox.tlsTerminate / sandbox.bwrapPath / sandbox.socatPath via
	// managed-settings.json so the SDK can observe them round-tripping through
	// the resolved-settings transport. tlsTerminate is experimental runtime
	// sandbox behavior, and bwrapPath/socatPath are honored only from
	// admin-controlled managed settings — none are surfaced on user CLI flag
	// entry points.
	t.Skip("not directly assertable from CLI: sandbox tlsTerminate is experimental runtime behavior, bwrapPath/socatPath are honored only from admin-controlled policy sources")
}

func TestIntegrationSettingsWorktreeFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI test fixture can deterministically inject
	// worktree.baseRef / worktree.bgIsolation via managed-settings.json so the
	// SDK can observe them round-tripping through the resolved-settings
	// transport. Both fields drive the CLI's --worktree / EnterWorktree flow,
	// which the Go SDK does not invoke directly.
	t.Skip("not directly assertable from CLI: worktree.baseRef and worktree.bgIsolation are consumed by CLI --worktree / EnterWorktree code paths the Go SDK does not invoke")
}

func TestIntegrationSettingsMiscFields(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill when the CLI test fixture can deterministically inject
	// statusLine.hideVimModeIndicator and the marketplace skills-dir /
	// unsupported / command source variants via managed-settings.json so the
	// SDK can observe them round-tripping through the resolved-settings
	// transport. Both fields are consumed by CLI renderer / registration code
	// paths the Go SDK does not invoke directly. The command variant has a
	// second blocker on top of that: the installed 2.1.222 binary predates it,
	// so it fails schema validation on the managed tier and stalls the
	// handshake rather than round-tripping.
	t.Skip("not directly assertable from CLI: statusLine.hideVimModeIndicator and marketplace skills-dir/unsupported/command source variants are consumed by CLI code paths the Go SDK does not invoke")
}

func TestIntegrationToolAliases(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	realCLI, err := DiscoverCLIPath(&Options{})
	require.NoError(t, err)

	tmp := t.TempDir()
	initLog := filepath.Join(tmp, "initialize.json")
	shimPath := filepath.Join(tmp, "claude-shim.py")
	shimBody := fmt.Sprintf(`#!/usr/bin/env python3
import json
import subprocess
import sys
import threading

init_log = %q
real_cli = %q

proc = subprocess.Popen(
    [real_cli] + sys.argv[1:],
    stdin=subprocess.PIPE,
    stdout=sys.stdout.buffer,
    stderr=sys.stderr.buffer,
)

captured = False

def forward_stdin():
    global captured
    try:
        for line in sys.stdin.buffer:
            if not captured:
                try:
                    msg = json.loads(line)
                    request = msg.get("request", {})
                    if (
                        msg.get("type") == "control_request"
                        and request.get("subtype") == "initialize"
                    ):
                        with open(init_log, "w", encoding="utf-8") as f:
                            json.dump(request, f)
                        captured = True
                except Exception:
                    pass
            proc.stdin.write(line)
            proc.stdin.flush()
    finally:
        proc.stdin.close()

thread = threading.Thread(target=forward_stdin)
thread.start()
code = proc.wait()
thread.join(timeout=1)
sys.exit(code)
`, initLog, realCLI)
	require.NoError(t, os.WriteFile(shimPath, []byte(shimBody), 0o755))

	opts := append(isolatedClientOptions(t),
		WithCLIPath(shimPath),
		WithToolAliases(map[string]string{"Bash": "Read"}),
	)

	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	stream, err := client.Stream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	init, err := stream.InitializationResult()
	require.NoError(t, err)
	require.NotNil(t, init)

	data, err := os.ReadFile(initLog)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, map[string]interface{}{"Bash": "Read"}, got["toolAliases"])
}

func TestIntegrationCompactBoundaryPreservedMessages(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)
	t.Skip("requires CLI to trigger a partial compaction that emits compact_metadata.preserved_messages (messagesToKeep populated); tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationMemoryRecallOrganizationScope(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)
	t.Skip("requires CLI to surface an organization-scoped memory (https URL + inline content); tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationPartialAssistantParentToolUseID(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)
	t.Skip("requires CLI to deterministically emit a streaming partial event nested under a Task-tool subagent (parent_tool_use_id non-null); tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationCommandsChangedMessage(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)
	t.Skip("not triggerable from CLI: commands_changed fires when the CLI dynamically discovers new skills mid-session, which requires modifying the skills directory while a session is live - not exercisable in standard integration runs; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationGetUsageExperimental(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)
	t.Skip("get_usage is EXPERIMENTAL upstream (unstable wire shape) and its response is account-dependent — rate_limits/behaviors are null for non-claude.ai-subscriber sessions and the installed CLI may not support the subtype; a live assertion would be brittle. Mirrors GetContextUsage, which ships without an integration test. Tracked in INTEGRATION-FOLLOWUPS.md")
}

// TestIntegrationSetMcpPermissionModeOverride exercises the
// set_mcp_permission_mode_override control request against the live CLI. The
// override targets an unknown server name, which a supporting CLI answers with
// a typo-detection warning (the override is stored regardless). CLIs predating
// the control request reject the subtype; the test skips in that case so the
// slot activates once the CLI catches up.
func TestIntegrationSetMcpPermissionModeOverride(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt("You are a helpful assistant. Be very brief."),
		WithMaxTurns(1),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := client.Stream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	mode := McpPermissionOverrideModeDefault
	result, err := stream.SetMcpPermissionModeOverride(ctx, "no-such-server", &mode)
	if err != nil {
		t.Skipf("CLI does not support set_mcp_permission_mode_override: %v", err)
	}

	// Unknown server name => informational typo-detection warning.
	assert.NotEmpty(t, result.Warning,
		"expected a warning for an unknown server name")

	// Clearing the override (nil mode => explicit wire null) must round-trip.
	_, err = stream.SetMcpPermissionModeOverride(ctx, "no-such-server", nil)
	require.NoError(t, err)
}

// TestIntegrationReinitialize exercises Stream.Reinitialize against the live
// CLI. Reinitialize re-sends the initialize control request to an
// already-initialized session and must return a fresh response. CLIs that
// reject a second initialize cause a clean skip.
func TestIntegrationReinitialize(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	opts := append(isolatedClientOptions(t),
		WithSystemPrompt("You are a helpful assistant. Be very brief."),
		WithMaxTurns(1),
	)
	client, err := NewClient(opts...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	stream, err := client.Stream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	resp, err := stream.Reinitialize(ctx)
	if err != nil {
		t.Skipf("CLI does not support reinitialize: %v", err)
	}
	require.NotNil(t, resp, "expected a fresh initialize response")
}

func TestIntegrationModelRefusalNoFallback(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)
	t.Skip("not triggerable from CLI: model_refusal_no_fallback requires the model to end a turn with stop_reason \"refusal\" while no fallback model is configured - a server-side refusal condition that cannot be deterministically reproduced in an integration run; tracked in INTEGRATION-FOLLOWUPS.md")
}

func TestIntegrationResumeDropsTurn(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	// TODO: Backfill once the installed CLI carries --resume-drops-turn. The
	// flag is absent from the 2.1.222 binary this suite runs against (the
	// neighboring --resume-session-at is present), so a live run would die on
	// an unknown option rather than exercise the fork-point guard. The
	// argv-shim pattern in TestIntegrationSettingsOptions is the shape to use:
	// build a session, fork at its last chain entry, and assert the CLI
	// refuses with "Resume rejected by --resume-drops-turn:" when an
	// unattributable entry sits in the discarded range.
	t.Skip("not triggerable from CLI: installed CLI 2.1.222 does not implement --resume-drops-turn; tracked in INTEGRATION-FOLLOWUPS.md")
}

// TestIntegrationAssistantContextUsage drives /context against the live CLI and
// checks the structured sibling on the assistant message that carries the
// markdown table. CLIs older than 2.1.233 answer /context with the table alone,
// so the assertions are conditional on the field being attached at all.
func TestIntegrationAssistantContextUsage(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	client, err := NewClient(isolatedClientOptions(t)...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var usage *AssistantContextUsage
	for msg := range client.Query(ctx, "/context") {
		assistant, ok := msg.(AssistantMessage)
		if !ok || assistant.ContextUsage == nil {
			continue
		}
		usage = assistant.ContextUsage
		break
	}

	if usage == nil {
		t.Skip("CLI did not attach context_usage to the /context result; " +
			"needs a CLI at 2.1.233 or newer")
	}

	assert.NotEmpty(t, usage.Model)
	assert.Positive(t, usage.TotalTokens)
	assert.Positive(t, usage.RawMaxTokens)
	require.NotEmpty(t, usage.Categories)

	// Every row must classify, and the rows that count toward the window have
	// to add up to the reported total. Deferred rows are out-of-window tool
	// schemas, so they are excluded from that sum.
	var inWindow int
	for _, cat := range usage.Categories {
		assert.Contains(t, []ContextUsageCategoryKind{
			ContextUsageUsed,
			ContextUsageFree,
			ContextUsageBuffer,
			ContextUsageDeferred,
		}, cat.Kind, "unclassified category row %q", cat.Name)

		if cat.Kind == ContextUsageUsed {
			inWindow += cat.Tokens
		}
	}
	assert.Equal(t, usage.TotalTokens, inWindow,
		"used rows should sum to total_tokens")

	if usage.OverLimit != nil {
		assert.Positive(t, usage.OverLimit.TokensOver)
		assert.Contains(t, []ContextWindowKind{
			ContextWindowHardLimit,
			ContextWindowCompaction,
		}, usage.OverLimit.Kind)
		assert.Greater(t, usage.TotalTokens, usage.RawMaxTokens,
			"over_limit set while within the window")
	}

	for _, tool := range usage.McpTools {
		assert.NotEmpty(t, tool.Name)
		assert.NotEmpty(t, tool.ServerName)
	}
	for _, file := range usage.MemoryFiles {
		assert.NotEmpty(t, file.Path)
	}
	for _, agent := range usage.Agents {
		assert.NotEmpty(t, agent.AgentType)
		assert.NotEmpty(t, agent.Source)
	}
}

// TestIntegrationInitTerminalSlashCommands checks the terminal-bound subset of
// the init message's advertised slash commands. CLIs older than 2.1.233 tag
// nothing, in which case the field is absent and a remote UI showing every
// command is the correct fallback.
func TestIntegrationInitTerminalSlashCommands(t *testing.T) {
	skipIfNoToken(t)
	skipIfNoCLI(t)

	client, err := NewClient(isolatedClientOptions(t)...)
	require.NoError(t, err)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var init *SystemMessage
	for msg := range client.Query(ctx, "Say hi") {
		system, ok := msg.(SystemMessage)
		if !ok || system.Subtype != "init" {
			continue
		}
		init = &system
		break
	}
	require.NotNil(t, init, "expected an init system message")
	require.NotEmpty(t, init.SlashCommands)

	if len(init.TerminalSlashCommands) == 0 {
		t.Skip("CLI advertised no terminal-bound slash commands; " +
			"needs a CLI at 2.1.233 or newer")
	}

	assert.Subset(t, init.SlashCommands, init.TerminalSlashCommands,
		"every terminal command must also appear in slash_commands")
}
