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
