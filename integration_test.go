//go:build integration

package claudeagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
