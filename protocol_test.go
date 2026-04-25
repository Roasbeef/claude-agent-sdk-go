package claudeagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProtocolInitialize tests the initialization flow.
func TestProtocolInitialize(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	// Add a hook
	opts.Hooks = map[HookType][]HookConfig{
		HookTypePreToolUse: {
			{
				Matcher: "*",
				Callback: func(ctx context.Context, input HookInput) (HookResult, error) {
					return HookResult{Continue: true}, nil
				},
			},
		},
	}

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Use channels to coordinate goroutines.
	readerReady := make(chan struct{})

	// Start a goroutine to handle incoming messages FIRST and signal when ready.
	go func() {
		close(readerReady)
		for msg, err := range transport.ReadMessages(ctx) {
			if err != nil {
				continue
			}
			if ctrlResp, ok := msg.(SDKControlResponse); ok {
				protocol.handleSDKControlResponse(ctrlResp)
			}
		}
	}()

	// Wait for reader to be ready before starting mock responder.
	<-readerReady

	// Send init response in background.
	go func() {
		// Read the init request from stdin (SDK format).
		decoder := json.NewDecoder(runner.StdinPipe)
		var initReq SDKControlRequest
		if err := decoder.Decode(&initReq); err != nil {
			return
		}

		// Write success response to stdout (SDK format).
		resp := SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: initReq.RequestID,
				Response:  map[string]interface{}{"status": "ok"},
			},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		runner.StdoutPipe.Write(data)
	}()

	// Run Initialize in a goroutine since io.Pipe is synchronous (Write blocks
	// until Read). This allows both sides of the pipe to run concurrently.
	initDone := make(chan error, 1)
	go func() {
		initDone <- protocol.Initialize(ctx)
	}()

	// Wait for Initialize to complete.
	select {
	case err = <-initDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("timeout waiting for Initialize to complete")
	}

	// Verify initialized
	assert.True(t, protocol.initialized.Load())

	// Second init should be no-op
	err = protocol.Initialize(ctx)
	require.NoError(t, err)
}

func TestProtocolInitializeOptions(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*Options)
		expected   map[string]interface{}
		unexpected []string
	}{
		{
			name: "all options",
			configure: func(opts *Options) {
				WithPlanModeInstructions("plan deliberately")(opts)
				WithTitle("session title")(opts)
				WithSkillsAllowlist([]string{"go", "review"})(opts)
				WithPromptSuggestions(false)(opts)
				WithAgentProgressSummaries(true)(opts)
				WithForwardSubagentText(false)(opts)
				WithExcludeDynamicSystemPromptSections(true)(opts)
			},
			expected: map[string]interface{}{
				"planModeInstructions":   "plan deliberately",
				"title":                  "session title",
				"skills":                 []interface{}{"go", "review"},
				"promptSuggestions":      false,
				"agentProgressSummaries": true,
				"forwardSubagentText":    false,
				"excludeDynamicSections": true,
			},
		},
		{
			name: "exclude dynamic false omitted",
			configure: func(opts *Options) {
				WithExcludeDynamicSystemPromptSections(false)(opts)
			},
			unexpected: []string{"excludeDynamicSections"},
		},
		{
			name: "agents wired through",
			configure: func(opts *Options) {
				WithAgents(map[string]AgentDefinition{
					"reviewer": {
						Description: "Reviews Go changes",
						Prompt:      "Review carefully",
						Tools:       []string{"Read", "Grep"},
					},
				})(opts)
			},
			expected: map[string]interface{}{
				"agents": map[string]interface{}{
					"reviewer": map[string]interface{}{
						"description": "Reviews Go changes",
						"prompt":      "Review carefully",
						"tools":       []interface{}{"Read", "Grep"},
					},
				},
			},
		},
		{
			name:       "no agents omits key",
			configure:  func(opts *Options) {},
			unexpected: []string{"agents"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewMockSubprocessRunner()
			opts := NewOptions()
			tt.configure(opts)

			transport := NewSubprocessTransportWithRunner(runner, opts)
			protocol := NewProtocol(transport, opts)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := transport.Connect(ctx)
			require.NoError(t, err)
			defer transport.Close()

			initReqCh := make(chan SDKControlRequest, 1)
			go func() {
				decoder := json.NewDecoder(runner.StdinPipe)
				var initReq SDKControlRequest
				if err := decoder.Decode(&initReq); err != nil {
					return
				}
				initReqCh <- initReq

				resp := SDKControlResponse{
					Type: "control_response",
					Response: SDKControlResponseBody{
						Subtype:   "success",
						RequestID: initReq.RequestID,
						Response:  map[string]interface{}{"status": "ok"},
					},
				}
				data, _ := json.Marshal(resp)
				data = append(data, '\n')
				runner.StdoutPipe.Write(data)
			}()

			go func() {
				for msg, err := range transport.ReadMessages(ctx) {
					if err != nil {
						continue
					}
					if ctrlResp, ok := msg.(SDKControlResponse); ok {
						protocol.handleSDKControlResponse(ctrlResp)
					}
				}
			}()

			err = protocol.Initialize(ctx)
			require.NoError(t, err)

			var initReq SDKControlRequest
			select {
			case initReq = <-initReqCh:
			case <-ctx.Done():
				t.Fatal("timeout waiting for initialize request")
			}

			data, err := json.Marshal(initReq.Request)
			require.NoError(t, err)
			var requestBody map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &requestBody))

			for key, want := range tt.expected {
				assert.Equal(t, want, requestBody[key])
			}
			for _, key := range tt.unexpected {
				assert.NotContains(t, requestBody, key)
			}
		})
	}
}

// TestProtocolPermissionRequest tests permission checking.
func TestProtocolPermissionRequest(t *testing.T) {
	t.Run("allow", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()

		// Set up permission callback that allows
		opts.CanUseTool = func(ctx context.Context, req ToolPermissionRequest) PermissionResult {
			assert.Equal(t, "fetch_quote", req.ToolName)
			return PermissionAllow{}
		}

		transport := NewSubprocessTransportWithRunner(runner, opts)
		protocol := NewProtocol(transport, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := transport.Connect(ctx)
		require.NoError(t, err)
		defer transport.Close()

		// Read response in background.
		respCh := make(chan SDKControlResponse, 1)
		go func() {
			decoder := json.NewDecoder(runner.StdinPipe)
			var resp SDKControlResponse
			if err := decoder.Decode(&resp); err == nil {
				respCh <- resp
			}
		}()

		// Simulate permission request from CLI (using TypeScript SDK format).
		req := ControlRequest{
			Type:      "control",
			Subtype:   "can_use_tool",
			RequestID: "req_1",
			Payload: map[string]interface{}{
				"tool_name":   "fetch_quote",
				"tool_use_id": "tool_1",
				"input": map[string]interface{}{
					"symbol": "AAPL",
				},
			},
		}

		// Handle the request.
		err = protocol.handleControlRequest(ctx, req)
		require.NoError(t, err)

		// Wait for response.
		select {
		case resp := <-respCh:
			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "success", resp.Response.Subtype)
			assert.Equal(t, "req_1", resp.Response.RequestID)
			assert.Equal(t, true, resp.Response.Response["allowed"])
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for response")
		}
	})

	t.Run("deny", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()

		// Set up permission callback that denies.
		opts.CanUseTool = func(ctx context.Context, req ToolPermissionRequest) PermissionResult {
			return PermissionDeny{Reason: "Tool not allowed in test mode"}
		}

		transport := NewSubprocessTransportWithRunner(runner, opts)
		protocol := NewProtocol(transport, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := transport.Connect(ctx)
		require.NoError(t, err)
		defer transport.Close()

		// Read response in background.
		respCh := make(chan SDKControlResponse, 1)
		go func() {
			decoder := json.NewDecoder(runner.StdinPipe)
			var resp SDKControlResponse
			if err := decoder.Decode(&resp); err == nil {
				respCh <- resp
			}
		}()

		// Simulate permission request (using TypeScript SDK format).
		req := ControlRequest{
			Type:      "control",
			Subtype:   "can_use_tool",
			RequestID: "req_2",
			Payload: map[string]interface{}{
				"tool_name":   "place_order",
				"tool_use_id": "tool_2",
				"input":       map[string]interface{}{},
			},
		}

		err = protocol.handleControlRequest(ctx, req)
		require.NoError(t, err)

		// Wait for response.
		select {
		case resp := <-respCh:
			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "success", resp.Response.Subtype)
			assert.Equal(t, "req_2", resp.Response.RequestID)
			assert.Equal(t, false, resp.Response.Response["allowed"])
			assert.Equal(t, "Tool not allowed in test mode", resp.Response.Response["reason"])
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for response")
		}
	})
}

// TestProtocolHookCallback tests hook invocation.
func TestProtocolHookCallback(t *testing.T) {
	t.Run("PreToolUse hook", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()

		// Track hook invocation.
		hookCalled := false

		opts.Hooks = map[HookType][]HookConfig{
			HookTypePreToolUse: {
				{
					Matcher: "*",
					Callback: func(ctx context.Context, input HookInput) (HookResult, error) {
						hookCalled = true
						preToolInput, ok := input.(PreToolUseInput)
						require.True(t, ok)
						assert.Equal(t, "fetch_quote", preToolInput.ToolName)
						return HookResult{Continue: true}, nil
					},
				},
			},
		}

		transport := NewSubprocessTransportWithRunner(runner, opts)
		protocol := NewProtocol(transport, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := transport.Connect(ctx)
		require.NoError(t, err)
		defer transport.Close()

		// Register hooks.
		protocol.hookCallbacks["hook_0"] = opts.Hooks[HookTypePreToolUse][0].Callback

		// Read response in background.
		respCh := make(chan SDKControlResponse, 1)
		go func() {
			decoder := json.NewDecoder(runner.StdinPipe)
			var resp SDKControlResponse
			if err := decoder.Decode(&resp); err == nil {
				respCh <- resp
			}
		}()

		// Simulate hook callback from CLI (using TypeScript SDK format).
		req := ControlRequest{
			Type:      "control",
			Subtype:   "hook_callback",
			RequestID: "req_hook_1",
			Payload: map[string]interface{}{
				"callback_id": "hook_0",
				"input": map[string]interface{}{
					"hook_event": "PreToolUse",
					"tool_name":  "fetch_quote",
					"tool_input": map[string]interface{}{
						"symbol": "AAPL",
					},
				},
			},
		}

		err = protocol.handleControlRequest(ctx, req)
		require.NoError(t, err)

		// Verify hook was called.
		assert.True(t, hookCalled)

		// Wait for response.
		select {
		case resp := <-respCh:
			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "success", resp.Response.Subtype)
			assert.Equal(t, "req_hook_1", resp.Response.RequestID)
			assert.Equal(t, true, resp.Response.Response["continue"])
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for response")
		}
	})
}

// TestProtocolSendMessage tests user message sending.
func TestProtocolSendMessage(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Use channels to coordinate goroutines.
	readerReady := make(chan struct{})
	responderReady := make(chan struct{})
	initResponseSent := make(chan struct{})
	userMsgReceived := make(chan UserMessage, 1)

	// Start message handler FIRST and signal when ready.
	go func() {
		close(readerReady)
		for msg, err := range transport.ReadMessages(ctx) {
			if err != nil {
				continue
			}
			if ctrlResp, ok := msg.(SDKControlResponse); ok {
				protocol.handleSDKControlResponse(ctrlResp)
			}
		}
	}()

	// Wait for reader to be ready before starting mock responder.
	<-readerReady

	// Mock responder - reads init request, sends response, then reads user message.
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		close(responderReady)

		// Read init request.
		var initReq SDKControlRequest
		if err := decoder.Decode(&initReq); err != nil {
			return
		}

		// Send init response.
		resp := SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: initReq.RequestID,
				Response:  map[string]interface{}{"status": "ok"},
			},
		}
		data, _ := json.Marshal(resp)
		data = append(data, '\n')
		runner.StdoutPipe.Write(data)
		close(initResponseSent)

		// Read user message.
		var userMsg UserMessage
		if err := decoder.Decode(&userMsg); err == nil {
			userMsgReceived <- userMsg
		}
	}()

	// Wait for responder to be ready before calling Initialize.
	select {
	case <-responderReady:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for responder to be ready")
	}

	// Run Initialize in a goroutine since io.Pipe is synchronous (Write blocks
	// until Read). This allows both sides of the pipe to run concurrently.
	initDone := make(chan error, 1)
	go func() {
		initDone <- protocol.Initialize(ctx)
	}()

	// Wait for Initialize to complete. The reader goroutine will route the
	// response from the mock responder to complete the initialization.
	select {
	case err = <-initDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("timeout waiting for Initialize to complete")
	}

	// Wait for init response to be sent before moving to next phase.
	select {
	case <-initResponseSent:
	case <-ctx.Done():
		t.Fatal("timeout waiting for init response sent")
	}

	// Verify initialized.
	assert.True(t, protocol.initialized.Load())

	// Send a user message.
	userMsg := UserMessage{
		Type:      "user",
		SessionID: "",
		Message: APIUserMessage{
			Role:    "user",
			Content: []UserContentBlock{{Type: "text", Text: "Hello Claude"}},
		},
	}

	err = protocol.SendMessage(ctx, userMsg)
	require.NoError(t, err)

	// Wait for user message to be received.
	select {
	case received := <-userMsgReceived:
		require.Len(t, received.Message.Content, 1)
		assert.Equal(t, "Hello Claude", received.Message.Content[0].Text)
	case <-ctx.Done():
		t.Fatal("timeout waiting for user message")
	}
}

// TestProtocolControlResponseRouting tests that responses are routed correctly.
func TestProtocolControlResponseRouting(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Simulate a pending request
	reqID := "test_req_123"
	respCh := make(chan ControlResponse, 1)
	protocol.pendingReqs.Store(reqID, respCh)

	// Send response
	resp := ControlResponse{
		Type:      "control",
		RequestID: reqID,
		Result:    map[string]interface{}{"data": "test"},
	}

	err = protocol.handleControlResponse(resp)
	require.NoError(t, err)

	// Verify response was routed
	select {
	case received := <-respCh:
		assert.Equal(t, reqID, received.RequestID)
		assert.Equal(t, "test", received.Result["data"])
	case <-time.After(1 * time.Second):
		t.Fatal("Response not received")
	}

	// Verify pending request was removed
	_, exists := protocol.pendingReqs.Load(reqID)
	assert.False(t, exists)
}

// TestProtocolConcurrentRequests tests thread-safety of request handling.
func TestProtocolConcurrentRequests(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx := context.Background()
	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Generate multiple request IDs concurrently
	numRequests := 100
	requestIDs := make([]string, numRequests)

	for i := 0; i < numRequests; i++ {
		requestIDs[i] = protocol.nextRequestID()
	}

	// Verify all IDs are unique
	idMap := make(map[string]bool)
	for _, id := range requestIDs {
		assert.False(t, idMap[id], "Duplicate request ID: %s", id)
		idMap[id] = true
	}

	assert.Len(t, idMap, numRequests)
}

// TestProtocolMCPMessage tests in-process MCP tool routing.
func TestProtocolMCPMessage(t *testing.T) {
	t.Run("tools/call success", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()

		// Create an in-process MCP server with a tool.
		server := CreateMcpServer(McpServerOptions{Name: "calculator"})
		type AddArgs struct {
			A int `json:"a"`
			B int `json:"b"`
		}
		AddTool(server, ToolDef{
			Name:        "add",
			Description: "Add two numbers",
		}, func(ctx context.Context, args AddArgs) (ToolResult, error) {
			return TextResult(string(rune('0' + args.A + args.B))), nil
		})

		// Register server in options.
		opts.SDKMcpServers = map[string]*McpServer{
			"calculator": server,
		}

		transport := NewSubprocessTransportWithRunner(runner, opts)
		protocol := NewProtocol(transport, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := transport.Connect(ctx)
		require.NoError(t, err)
		defer transport.Close()

		// Read response in background.
		respCh := make(chan SDKControlResponse, 1)
		go func() {
			decoder := json.NewDecoder(runner.StdinPipe)
			var resp SDKControlResponse
			if err := decoder.Decode(&resp); err == nil {
				respCh <- resp
			}
		}()

		// Simulate mcp_message request from CLI.
		req := ControlRequest{
			Type:      "control",
			Subtype:   "mcp_message",
			RequestID: "req_mcp_1",
			Payload: map[string]interface{}{
				"server_name": "calculator",
				"message_id":  "msg_1",
				"message": map[string]interface{}{
					"method": "tools/call",
					"params": map[string]interface{}{
						"name": "add",
						"arguments": map[string]interface{}{
							"a": 3,
							"b": 5,
						},
					},
				},
			},
		}

		err = protocol.handleControlRequest(ctx, req)
		require.NoError(t, err)

		// Wait for response.
		select {
		case resp := <-respCh:
			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "success", resp.Response.Subtype)
			assert.Equal(t, "req_mcp_1", resp.Response.RequestID)
			// Check the result contains the expected content.
			result, ok := resp.Response.Response["result"].(map[string]interface{})
			require.True(t, ok, "result should be a map")
			content, ok := result["content"].([]interface{})
			require.True(t, ok, "content should be an array")
			require.Len(t, content, 1)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for response")
		}
	})

	t.Run("tools/list", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()

		// Create an in-process MCP server with multiple tools.
		server := CreateMcpServer(McpServerOptions{Name: "mytools"})
		AddToolUntyped(server, ToolDef{
			Name:        "tool1",
			Description: "First tool",
		}, func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
			return TextResult("ok"), nil
		})
		AddToolUntyped(server, ToolDef{
			Name:        "tool2",
			Description: "Second tool",
		}, func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
			return TextResult("ok"), nil
		})

		opts.SDKMcpServers = map[string]*McpServer{
			"mytools": server,
		}

		transport := NewSubprocessTransportWithRunner(runner, opts)
		protocol := NewProtocol(transport, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := transport.Connect(ctx)
		require.NoError(t, err)
		defer transport.Close()

		// Read response in background.
		respCh := make(chan SDKControlResponse, 1)
		go func() {
			decoder := json.NewDecoder(runner.StdinPipe)
			var resp SDKControlResponse
			if err := decoder.Decode(&resp); err == nil {
				respCh <- resp
			}
		}()

		// Simulate mcp_message request for tools/list.
		req := ControlRequest{
			Type:      "control",
			Subtype:   "mcp_message",
			RequestID: "req_mcp_2",
			Payload: map[string]interface{}{
				"server_name": "mytools",
				"message_id":  "msg_2",
				"message": map[string]interface{}{
					"method": "tools/list",
					"params": map[string]interface{}{},
				},
			},
		}

		err = protocol.handleControlRequest(ctx, req)
		require.NoError(t, err)

		// Wait for response.
		select {
		case resp := <-respCh:
			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "success", resp.Response.Subtype)
			result, ok := resp.Response.Response["result"].(map[string]interface{})
			require.True(t, ok, "result should be a map")
			tools, ok := result["tools"].([]interface{})
			require.True(t, ok, "tools should be an array")
			assert.Len(t, tools, 2)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for response")
		}
	})

	t.Run("unknown server", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()

		transport := NewSubprocessTransportWithRunner(runner, opts)
		protocol := NewProtocol(transport, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := transport.Connect(ctx)
		require.NoError(t, err)
		defer transport.Close()

		// Read response in background.
		respCh := make(chan SDKControlResponse, 1)
		go func() {
			decoder := json.NewDecoder(runner.StdinPipe)
			var resp SDKControlResponse
			if err := decoder.Decode(&resp); err == nil {
				respCh <- resp
			}
		}()

		// Simulate mcp_message request for unknown server.
		req := ControlRequest{
			Type:      "control",
			Subtype:   "mcp_message",
			RequestID: "req_mcp_3",
			Payload: map[string]interface{}{
				"server_name": "nonexistent",
				"message_id":  "msg_3",
				"message": map[string]interface{}{
					"method": "tools/call",
					"params": map[string]interface{}{},
				},
			},
		}

		err = protocol.handleControlRequest(ctx, req)
		require.NoError(t, err)

		// Wait for error response.
		select {
		case resp := <-respCh:
			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "error", resp.Response.Subtype)
			assert.Contains(t, resp.Response.Error, "unknown MCP server")
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for response")
		}
	})
}

// TestProtocolSDKMCPMessage tests in-process MCP tool routing via SDK control format.
// This tests the actual format the CLI sends (SDKControlRequest, not legacy ControlRequest).
func TestProtocolSDKMCPMessage(t *testing.T) {
	t.Run("tools/call via SDK format", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()

		// Create an in-process MCP server with a tool.
		server := CreateMcpServer(McpServerOptions{Name: "calculator"})
		type AddArgs struct {
			A int `json:"a"`
			B int `json:"b"`
		}
		AddTool(server, ToolDef{
			Name:        "add",
			Description: "Add two numbers",
		}, func(ctx context.Context, args AddArgs) (ToolResult, error) {
			sum := args.A + args.B
			return TextResult(string(rune('0' + sum))), nil
		})

		opts.SDKMcpServers = map[string]*McpServer{
			"calculator": server,
		}

		transport := NewSubprocessTransportWithRunner(runner, opts)
		protocol := NewProtocol(transport, opts)

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := transport.Connect(ctx)
		require.NoError(t, err)
		defer transport.Close()

		// Read response in background.
		respCh := make(chan SDKControlResponse, 1)
		go func() {
			decoder := json.NewDecoder(runner.StdinPipe)
			var resp SDKControlResponse
			if err := decoder.Decode(&resp); err == nil {
				respCh <- resp
			}
		}()

		// Simulate mcp_message request from CLI using SDK format.
		req := SDKControlRequest{
			Type:      "control_request",
			RequestID: "sdk_mcp_1",
			Request: SDKControlRequestBody{
				Subtype:    "mcp_message",
				ServerName: "calculator",
				Message: map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      "msg_1",
					"method":  "tools/call",
					"params": map[string]interface{}{
						"name": "add",
						"arguments": map[string]interface{}{
							"a": 3,
							"b": 5,
						},
					},
				},
			},
		}

		err = protocol.handleSDKControlRequest(ctx, req)
		require.NoError(t, err)

		// Wait for response.
		select {
		case resp := <-respCh:
			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "success", resp.Response.Subtype)
			assert.Equal(t, "sdk_mcp_1", resp.Response.RequestID)

			// Response should be wrapped in mcp_response field.
			mcpResponse, ok := resp.Response.Response["mcp_response"].(map[string]interface{})
			require.True(t, ok, "mcp_response should be a map")

			// Check JSONRPC format inside mcp_response.
			assert.Equal(t, "2.0", mcpResponse["jsonrpc"])
			assert.Equal(t, "msg_1", mcpResponse["id"])

			// Check the result.
			result, ok := mcpResponse["result"].(map[string]interface{})
			require.True(t, ok, "result should be a map")
			content, ok := result["content"].([]interface{})
			require.True(t, ok, "content should be an array")
			require.Len(t, content, 1)
		case <-time.After(500 * time.Millisecond):
			t.Fatal("Timeout waiting for response")
		}
	})
}

// TestBuildHookResponse_StopHookOmitsContinue verifies that when a Stop
// hook returns Decision="block", the serialized response does NOT include
// a "continue" field. Shell-based stop hooks output {"decision":"block",
// "reason":"..."} without "continue", and including "continue":false
// causes the CLI to short-circuit and terminate the session before
// honoring the block decision.
func TestBuildHookResponse_StopHookOmitsContinue(t *testing.T) {
	t.Run("block decision omits continue", func(t *testing.T) {
		result := HookResult{
			Decision:      "block",
			Reason:        "Re-review feedback from author",
			SystemMessage: "You have 1 unread message",
		}

		resp := buildHookResponse("Stop", result)

		// Must have decision, reason, systemMessage.
		assert.Equal(t, "block", resp["decision"])
		assert.Equal(t,
			"Re-review feedback from author", resp["reason"],
		)
		assert.Equal(t,
			"You have 1 unread message",
			resp["systemMessage"],
		)

		// Must NOT have "continue" — shell hooks never emit it
		// for stop hooks, and including it causes the CLI to
		// terminate before processing the injected prompt.
		_, hasContinue := resp["continue"]
		assert.False(t, hasContinue,
			"stop hook block response must not include "+
				"'continue' field",
		)
	})

	t.Run("approve decision omits continue", func(t *testing.T) {
		result := HookResult{
			Decision: "approve",
		}

		resp := buildHookResponse("Stop", result)

		assert.Equal(t, "approve", resp["decision"])

		_, hasContinue := resp["continue"]
		assert.False(t, hasContinue,
			"stop hook approve response must not include "+
				"'continue' field",
		)
	})

	t.Run("non-stop hook includes continue", func(t *testing.T) {
		// PreToolUse hooks use Continue, not Decision.
		result := HookResult{
			Continue: true,
		}

		resp := buildHookResponse("PreToolUse", result)

		assert.Equal(t, true, resp["continue"])

		// Must NOT have decision fields.
		_, hasDecision := resp["decision"]
		assert.False(t, hasDecision,
			"non-stop hook should not include decision",
		)
	})

	t.Run("block with modify uses legacy format", func(t *testing.T) {
		// Stop hooks with Modify should use the legacy modify
		// field since Stop is not PreToolUse or PermissionRequest.
		result := HookResult{
			Decision: "block",
			Reason:   "New task",
			Modify: map[string]interface{}{
				"key": "value",
			},
		}

		resp := buildHookResponse("Stop", result)

		assert.Equal(t, "block", resp["decision"])
		assert.Equal(t, "New task", resp["reason"])

		// Modify should still be included as legacy format.
		modify, ok := resp["modify"]
		assert.True(t, ok)
		assert.Equal(t,
			map[string]interface{}{"key": "value"}, modify,
		)

		// Continue must still be omitted.
		_, hasContinue := resp["continue"]
		assert.False(t, hasContinue)
	})
}

// TestHandleHookCallback_PreToolUseModify exercises the full
// handleHookCallback path for a PreToolUse hook that returns Modify.
// This verifies the hookType is correctly extracted from the legacy
// control request payload and threaded through to buildHookResponse,
// producing hookSpecificOutput.updatedInput on the wire.
func TestHandleHookCallback_PreToolUseModify(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	opts.Hooks = map[HookType][]HookConfig{
		HookTypePreToolUse: {
			{
				Matcher: "*",
				Callback: func(ctx context.Context, input HookInput) (HookResult, error) {
					ptu, ok := input.(PreToolUseInput)
					require.True(t, ok)
					assert.Equal(t, "Bash", ptu.ToolName)

					return HookResult{
						Continue: true,
						Modify: map[string]interface{}{
							"command": "cd /worktree && " + "git status",
						},
					}, nil
				},
			},
		},
	}

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	// Register hook callback (normally done during Initialize).
	protocol.hookCallbacks["hook_ptu_0"] = opts.Hooks[HookTypePreToolUse][0].Callback

	// Read the response that handleControlRequest writes to the transport.
	respCh := make(chan SDKControlResponse, 1)
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		var resp SDKControlResponse
		if err := decoder.Decode(&resp); err == nil {
			respCh <- resp
		}
	}()

	// Simulate a PreToolUse hook callback from the CLI (legacy format).
	req := ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_ptu_modify",
		Payload: map[string]interface{}{
			"callback_id": "hook_ptu_0",
			"input": map[string]interface{}{
				"hook_event": "PreToolUse",
				"tool_name":  "Bash",
				"tool_input": map[string]interface{}{
					"command": "git status",
				},
				"session_id": "sess_1",
			},
		},
	}

	err = protocol.handleControlRequest(ctx, req)
	require.NoError(t, err)

	select {
	case resp := <-respCh:
		assert.Equal(t, "control_response", resp.Type)
		assert.Equal(t, "success", resp.Response.Subtype)
		assert.Equal(t, "req_ptu_modify", resp.Response.RequestID)

		// Wire format must use hookSpecificOutput, not legacy modify.
		_, hasModify := resp.Response.Response["modify"]
		assert.False(t, hasModify,
			"PreToolUse response must not use legacy modify field",
		)

		hso, ok := resp.Response.Response["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok, "response must include hookSpecificOutput")
		assert.Equal(t, "PreToolUse", hso["hookEventName"])
		assert.Equal(t, "allow", hso["permissionDecision"])

		updatedInput, ok := hso["updatedInput"].(map[string]interface{})
		require.True(t, ok, "hookSpecificOutput must include updatedInput")
		assert.Equal(t, "cd /worktree && git status", updatedInput["command"])

	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

// TestHandleSDKHookCallback_PreToolUseModify exercises the SDK-format
// handleSDKHookCallback path for a PreToolUse hook that returns Modify.
// The SDK format uses hook_event_name (not hook_event) in the Input map,
// and the response must use hookSpecificOutput.updatedInput.
func TestHandleSDKHookCallback_PreToolUseModify(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	opts.Hooks = map[HookType][]HookConfig{
		HookTypePreToolUse: {
			{
				Matcher: "*",
				Callback: func(ctx context.Context, input HookInput) (HookResult, error) {
					ptu := input.(PreToolUseInput)
					return HookResult{
						Continue: true,
						Modify: map[string]interface{}{
							"file_path": "/worktree/" + ptu.ToolName,
						},
					}, nil
				},
			},
		},
	}

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	protocol.hookCallbacks["sdk_hook_ptu"] = opts.Hooks[HookTypePreToolUse][0].Callback

	// Read the response written to the transport.
	respCh := make(chan SDKControlResponse, 1)
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		var resp SDKControlResponse
		if err := decoder.Decode(&resp); err == nil {
			respCh <- resp
		}
	}()

	// Simulate a PreToolUse hook callback in SDK format.
	req := SDKControlRequest{
		Type:      "control_request",
		RequestID: "sdk_ptu_modify",
		Request: SDKControlRequestBody{
			Subtype:    "hook_callback",
			CallbackID: "sdk_hook_ptu",
			Input: map[string]interface{}{
				"hook_event_name": "PreToolUse",
				"tool_name":       "Read",
				"tool_input": map[string]interface{}{
					"file_path": "/old/path.go",
				},
				"session_id": "sess_sdk_1",
			},
		},
	}

	err = protocol.handleSDKControlRequest(ctx, req)
	require.NoError(t, err)

	select {
	case resp := <-respCh:
		assert.Equal(t, "control_response", resp.Type)
		assert.Equal(t, "success", resp.Response.Subtype)
		assert.Equal(t, "sdk_ptu_modify", resp.Response.RequestID)

		_, hasModify := resp.Response.Response["modify"]
		assert.False(t, hasModify,
			"SDK PreToolUse response must not use legacy modify",
		)

		hso, ok := resp.Response.Response["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok, "response must include hookSpecificOutput")
		assert.Equal(t, "PreToolUse", hso["hookEventName"])
		assert.Equal(t, "allow", hso["permissionDecision"])

		updatedInput, ok := hso["updatedInput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "/worktree/Read", updatedInput["file_path"])

	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

func TestHandleHookCallback_BaseAgentFields(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["hook_base_agent"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		preToolInput, ok := input.(PreToolUseInput)
		require.True(t, ok)
		assert.Equal(t, "agent_123", preToolInput.Base().AgentID)
		assert.Equal(t, "reviewer", preToolInput.Base().AgentType)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleHookCallback(context.Background(), ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_base_agent",
		Payload: map[string]interface{}{
			"callback_id": "hook_base_agent",
			"input": map[string]interface{}{
				"hook_event": "PreToolUse",
				"tool_name":  "Read",
				"tool_input": map[string]interface{}{
					"file_path": "README.md",
				},
				"agent_id":   "agent_123",
				"agent_type": "reviewer",
			},
		},
	})

	assert.Equal(t, "success", resp.Response.Subtype)
	assert.Equal(t, "req_base_agent", resp.Response.RequestID)
}

func TestHandleHookCallback_StopInputFields(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["hook_stop"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		stopInput, ok := input.(StopInput)
		require.True(t, ok)
		assert.True(t, stopInput.StopHookActive)
		assert.Equal(t, "final answer", stopInput.LastAssistantMessage)
		assert.Equal(t, "agent_stop", stopInput.Base().AgentID)
		assert.Equal(t, "planner", stopInput.Base().AgentType)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleHookCallback(context.Background(), ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_stop",
		Payload: map[string]interface{}{
			"callback_id": "hook_stop",
			"input": map[string]interface{}{
				"hook_event":             "Stop",
				"stop_hook_active":       true,
				"last_assistant_message": "final answer",
				"agent_id":               "agent_stop",
				"agent_type":             "planner",
			},
		},
	})

	assert.Equal(t, "success", resp.Response.Subtype)
	assert.Equal(t, "req_stop", resp.Response.RequestID)
}

func TestHandleSDKHookCallback_SubagentStopInputFields(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["sdk_subagent_stop"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		subagentStopInput, ok := input.(SubagentStopInput)
		require.True(t, ok)
		assert.True(t, subagentStopInput.StopHookActive)
		assert.Equal(t, "/tmp/agent-transcript.jsonl", subagentStopInput.AgentTranscriptPath)
		assert.Equal(t, "subagent answer", subagentStopInput.LastAssistantMessage)
		assert.Equal(t, "legacy-name", subagentStopInput.AgentName)
		assert.Equal(t, "done", subagentStopInput.Status)
		assert.Equal(t, "legacy result", subagentStopInput.Result)
		assert.Equal(t, "agent_sdk", subagentStopInput.Base().AgentID)
		assert.Equal(t, "builder", subagentStopInput.Base().AgentType)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
		Type:      "control_request",
		RequestID: "sdk_subagent_stop",
		Request: SDKControlRequestBody{
			Subtype:    "hook_callback",
			CallbackID: "sdk_subagent_stop",
			Input: map[string]interface{}{
				"hook_event_name":        "SubagentStop",
				"stop_hook_active":       true,
				"agent_id":               "agent_sdk",
				"agent_transcript_path":  "/tmp/agent-transcript.jsonl",
				"agent_type":             "builder",
				"last_assistant_message": "subagent answer",
				"agent_name":             "legacy-name",
				"status":                 "done",
				"result":                 "legacy result",
			},
		},
	})

	assert.Equal(t, "success", resp.Response.Subtype)
	assert.Equal(t, "sdk_subagent_stop", resp.Response.RequestID)
}

func TestHandleSDKHookCallback_MissingHookAuditFields(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["sdk_subagent_stop_legacy"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		subagentStopInput, ok := input.(SubagentStopInput)
		require.True(t, ok)
		assert.False(t, subagentStopInput.StopHookActive)
		assert.Empty(t, subagentStopInput.AgentTranscriptPath)
		assert.Empty(t, subagentStopInput.LastAssistantMessage)
		assert.Empty(t, subagentStopInput.Base().AgentID)
		assert.Empty(t, subagentStopInput.Base().AgentType)
		assert.Equal(t, "legacy-name", subagentStopInput.AgentName)
		assert.Equal(t, "done", subagentStopInput.Status)
		assert.Equal(t, "legacy result", subagentStopInput.Result)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
		Type:      "control_request",
		RequestID: "sdk_subagent_stop_legacy",
		Request: SDKControlRequestBody{
			Subtype:    "hook_callback",
			CallbackID: "sdk_subagent_stop_legacy",
			Input: map[string]interface{}{
				"hook_event_name": "SubagentStop",
				"agent_name":      "legacy-name",
				"status":          "done",
				"result":          "legacy result",
			},
		},
	})

	assert.Equal(t, "success", resp.Response.Subtype)
	assert.Equal(t, "sdk_subagent_stop_legacy", resp.Response.RequestID)
}

// TestHandleSDKHookCallback_PermissionRequestModify verifies the SDK
// format path for PermissionRequest hooks with Modify, which uses a
// nested decision.updatedInput structure.
func TestHandleSDKHookCallback_PermissionRequestModify(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	opts.Hooks = map[HookType][]HookConfig{
		HookTypePermissionRequest: {
			{
				Matcher: "*",
				Callback: func(ctx context.Context, input HookInput) (HookResult, error) {
					return HookResult{
						Continue: true,
						Modify: map[string]interface{}{
							"command": "safe-command --flag",
						},
					}, nil
				},
			},
		},
	}

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	protocol.hookCallbacks["sdk_hook_pr"] = opts.Hooks[HookTypePermissionRequest][0].Callback

	respCh := make(chan SDKControlResponse, 1)
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		var resp SDKControlResponse
		if err := decoder.Decode(&resp); err == nil {
			respCh <- resp
		}
	}()

	req := SDKControlRequest{
		Type:      "control_request",
		RequestID: "sdk_pr_modify",
		Request: SDKControlRequestBody{
			Subtype:    "hook_callback",
			CallbackID: "sdk_hook_pr",
			Input: map[string]interface{}{
				"hook_event_name": "PermissionRequest",
				"tool_name":       "Bash",
				"tool_input": map[string]interface{}{
					"command": "rm -rf /",
				},
				"session_id": "sess_sdk_2",
			},
		},
	}

	err = protocol.handleSDKControlRequest(ctx, req)
	require.NoError(t, err)

	select {
	case resp := <-respCh:
		assert.Equal(t, "success", resp.Response.Subtype)

		_, hasModify := resp.Response.Response["modify"]
		assert.False(t, hasModify)

		hso, ok := resp.Response.Response["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "PermissionRequest", hso["hookEventName"])

		decision, ok := hso["decision"].(map[string]interface{})
		require.True(t, ok, "PermissionRequest must use nested decision")
		assert.Equal(t, "allow", decision["behavior"])

		updatedInput, ok := decision["updatedInput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "safe-command --flag", updatedInput["command"])

	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

// TestHandleHookCallback_EmptyHookType verifies that when a hook callback
// arrives without a hook_event field (empty string hookType), Modify falls
// through to the legacy format rather than producing hookSpecificOutput.
func TestHandleHookCallback_EmptyHookType(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	// Use a UserPromptSubmit hook but simulate a missing hook_event field.
	// The callback registered under a generic ID will fire, and
	// buildHookResponse will receive hookType="" which should fall to default.
	opts.Hooks = map[HookType][]HookConfig{
		HookTypeUserPromptSubmit: {
			{
				Matcher: "*",
				Callback: func(ctx context.Context, input HookInput) (HookResult, error) {
					// Without a recognized hook_event, handleHookCallback
					// falls to default and returns an error. So this won't
					// be called. We test the error path instead.
					return HookResult{Continue: true}, nil
				},
			},
		},
	}

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	protocol.hookCallbacks["hook_empty"] = opts.Hooks[HookTypeUserPromptSubmit][0].Callback

	respCh := make(chan SDKControlResponse, 1)
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		var resp SDKControlResponse
		if err := decoder.Decode(&resp); err == nil {
			respCh <- resp
		}
	}()

	// Send a hook callback with NO hook_event field in the input.
	req := ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_empty_type",
		Payload: map[string]interface{}{
			"callback_id": "hook_empty",
			"input": map[string]interface{}{
				// hook_event intentionally omitted.
				"session_id": "sess_empty",
			},
		},
	}

	err = protocol.handleControlRequest(ctx, req)
	require.NoError(t, err)

	select {
	case resp := <-respCh:
		// With an empty/missing hook_event, the switch in
		// handleHookCallback falls to default, returning an error
		// about an unknown hook type.
		assert.Equal(t, "error", resp.Response.Subtype)
		assert.Contains(t, resp.Response.Error, "unknown hook type")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

// TestHandleHookCallback_HookSpecificOutputPassthrough verifies that a
// hook returning HookSpecificOutput directly passes it through the full
// handleHookCallback → buildHookResponse → wire path unchanged.
func TestHandleHookCallback_HookSpecificOutputPassthrough(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	opts.Hooks = map[HookType][]HookConfig{
		HookTypePreToolUse: {
			{
				Matcher: "*",
				Callback: func(ctx context.Context, input HookInput) (HookResult, error) {
					return HookResult{
						Continue: true,
						HookSpecificOutput: map[string]interface{}{
							"hookEventName":            "PreToolUse",
							"permissionDecision":       "deny",
							"permissionDecisionReason": "blocked by policy",
						},
					}, nil
				},
			},
		},
	}

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := transport.Connect(ctx)
	require.NoError(t, err)
	defer transport.Close()

	protocol.hookCallbacks["hook_hso"] = opts.Hooks[HookTypePreToolUse][0].Callback

	respCh := make(chan SDKControlResponse, 1)
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		var resp SDKControlResponse
		if err := decoder.Decode(&resp); err == nil {
			respCh <- resp
		}
	}()

	req := ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_hso_pass",
		Payload: map[string]interface{}{
			"callback_id": "hook_hso",
			"input": map[string]interface{}{
				"hook_event": "PreToolUse",
				"tool_name":  "Bash",
				"tool_input": map[string]interface{}{
					"command": "rm -rf /",
				},
			},
		},
	}

	err = protocol.handleControlRequest(ctx, req)
	require.NoError(t, err)

	select {
	case resp := <-respCh:
		assert.Equal(t, "success", resp.Response.Subtype)

		hso, ok := resp.Response.Response["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "deny", hso["permissionDecision"])
		assert.Equal(t, "blocked by policy", hso["permissionDecisionReason"])

		// No legacy modify field should be present.
		_, hasModify := resp.Response.Response["modify"]
		assert.False(t, hasModify)

	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for response")
	}
}

// TestBuildHookResponse_PreToolUseUpdatedInput verifies that PreToolUse
// hooks with Modify produce hookSpecificOutput.updatedInput instead of
// the legacy modify field. The CLI ignores the modify field; only
// hookSpecificOutput.updatedInput actually rewrites tool inputs.
func TestBuildHookResponse_PreToolUseUpdatedInput(t *testing.T) {
	t.Run("modify translates to updatedInput", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Modify: map[string]interface{}{
				"command": "cd /tmp/worktree && git status",
			},
		}

		resp := buildHookResponse("PreToolUse", result)

		// Must have continue=true.
		assert.Equal(t, true, resp["continue"])

		// Must NOT have legacy modify field.
		_, hasModify := resp["modify"]
		assert.False(t, hasModify,
			"PreToolUse should not use legacy modify field",
		)

		// Must have hookSpecificOutput with updatedInput.
		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok, "hookSpecificOutput should be a map")
		assert.Equal(t, "PreToolUse", hso["hookEventName"])
		assert.Equal(t, "allow", hso["permissionDecision"])

		updatedInput, ok := hso["updatedInput"].(map[string]interface{})
		require.True(t, ok, "updatedInput should be a map")
		assert.Equal(t,
			"cd /tmp/worktree && git status",
			updatedInput["command"],
		)
	})

	t.Run("file_path modification", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Modify: map[string]interface{}{
				"file_path": "/tmp/worktree/src/main.go",
			},
		}

		resp := buildHookResponse("PreToolUse", result)

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)

		updatedInput, ok := hso["updatedInput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t,
			"/tmp/worktree/src/main.go",
			updatedInput["file_path"],
		)
	})

	t.Run("no modify produces no hookSpecificOutput", func(t *testing.T) {
		result := HookResult{
			Continue: true,
		}

		resp := buildHookResponse("PreToolUse", result)

		assert.Equal(t, true, resp["continue"])

		_, hasHSO := resp["hookSpecificOutput"]
		assert.False(t, hasHSO,
			"no modify should produce no hookSpecificOutput",
		)

		_, hasModify := resp["modify"]
		assert.False(t, hasModify)
	})

	t.Run("PermissionRequest uses nested decision format", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Modify: map[string]interface{}{
				"command": "ls /tmp",
			},
		}

		resp := buildHookResponse("PermissionRequest", result)

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "PermissionRequest", hso["hookEventName"])

		decision, ok := hso["decision"].(map[string]interface{})
		require.True(t, ok, "decision should be a map")
		assert.Equal(t, "allow", decision["behavior"])

		updatedInput, ok := decision["updatedInput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ls /tmp", updatedInput["command"])
	})

	t.Run("PostToolUse falls through to legacy modify", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Modify: map[string]interface{}{
				"key": "value",
			},
		}

		resp := buildHookResponse("PostToolUse", result)

		// Should use legacy modify field for non-PreToolUse hooks.
		modify, ok := resp["modify"].(map[string]interface{})
		require.True(t, ok, "PostToolUse should use legacy modify")
		assert.Equal(t, "value", modify["key"])

		_, hasHSO := resp["hookSpecificOutput"]
		assert.False(t, hasHSO,
			"PostToolUse should not use hookSpecificOutput",
		)
	})

	t.Run("explicit HookSpecificOutput takes precedence", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Modify: map[string]interface{}{
				"command": "should be ignored",
			},
			HookSpecificOutput: map[string]interface{}{
				"hookEventName":      "PreToolUse",
				"permissionDecision": "deny",
				"permissionDecisionReason": "blocked by " +
					"policy",
			},
		}

		resp := buildHookResponse("PreToolUse", result)

		// HookSpecificOutput should be used as-is.
		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "deny", hso["permissionDecision"])
		assert.Equal(t,
			"blocked by policy",
			hso["permissionDecisionReason"],
		)

		// Legacy modify should NOT be present.
		_, hasModify := resp["modify"]
		assert.False(t, hasModify)
	})
}

func TestBuildHookResponse_WatchPaths(t *testing.T) {
	t.Run("CwdChanged emits watchPaths", func(t *testing.T) {
		resp := buildHookResponse("CwdChanged", HookResult{
			Continue:   true,
			WatchPaths: []string{"/foo", "/bar"},
		})

		assert.Equal(t, true, resp["continue"])
		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "CwdChanged", hso["hookEventName"])
		assert.Equal(t, []string{"/foo", "/bar"}, hso["watchPaths"])
	})

	t.Run("WorktreeCreate preserves explicit output", func(t *testing.T) {
		result := HookResult{
			Continue:   true,
			WatchPaths: []string{"/x"},
			HookSpecificOutput: map[string]interface{}{
				"hookEventName": "WorktreeCreate",
				"worktreePath":  "/x",
			},
		}

		resp := buildHookResponse("WorktreeCreate", result)

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "WorktreeCreate", hso["hookEventName"])
		assert.Equal(t, "/x", hso["worktreePath"])
		assert.Equal(t, []string{"/x"}, hso["watchPaths"])

		_, mutated := result.HookSpecificOutput["watchPaths"]
		assert.False(t, mutated)
	})

	t.Run("FileChanged empty WatchPaths omits hookSpecificOutput", func(t *testing.T) {
		resp := buildHookResponse("FileChanged", HookResult{
			Continue: true,
		})

		_, hasHSO := resp["hookSpecificOutput"]
		assert.False(t, hasHSO)
	})

	t.Run("PreToolUse does not emit watchPaths", func(t *testing.T) {
		resp := buildHookResponse("PreToolUse", HookResult{
			Continue:   true,
			WatchPaths: []string{"/never"},
		})

		_, hasHSO := resp["hookSpecificOutput"]
		assert.False(t, hasHSO)
	})

	t.Run("SessionStart emits watchPaths with continue", func(t *testing.T) {
		resp := buildHookResponse("SessionStart", HookResult{
			Continue:   true,
			WatchPaths: []string{"/cfg"},
		})

		assert.Equal(t, true, resp["continue"])
		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "SessionStart", hso["hookEventName"])
		assert.Equal(t, []string{"/cfg"}, hso["watchPaths"])
	})
}

// TestHandleHookCallback_ShapeCompatibleEvents covers the 12 v0.2.119 events
// added in PR 8b. Each subtest exercises one event end-to-end through one of
// the two dispatch paths and asserts every event-specific field on the parsed
// input.
func TestHandleHookCallback_ShapeCompatibleEvents(t *testing.T) {
	t.Run("ConfigChange via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(ConfigChangeInput)
			require.True(t, ok)
			assert.Equal(t, "user_settings", ev.Source)
			assert.Equal(t, "/home/u/.claude/settings.json", ev.FilePath)
			assert.Equal(t, "agent-x", ev.Base().AgentID)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event": "ConfigChange",
					"source":     "user_settings",
					"file_path":  "/home/u/.claude/settings.json",
					"agent_id":   "agent-x",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("InstructionsLoaded via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(InstructionsLoadedInput)
			require.True(t, ok)
			assert.Equal(t, "/proj/CLAUDE.md", ev.FilePath)
			assert.Equal(t, "Project", ev.MemoryType)
			assert.Equal(t, "session_start", ev.LoadReason)
			assert.Equal(t, []string{"**/*.md"}, ev.Globs)
			assert.Equal(t, "/proj/parent.md", ev.ParentFilePath)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name":  "InstructionsLoaded",
					"file_path":        "/proj/CLAUDE.md",
					"memory_type":      "Project",
					"load_reason":      "session_start",
					"globs":            []interface{}{"**/*.md"},
					"parent_file_path": "/proj/parent.md",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("PostCompact via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(PostCompactInput)
			require.True(t, ok)
			assert.Equal(t, "auto", ev.Trigger)
			assert.Equal(t, "summary text", ev.CompactSummary)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event":      "PostCompact",
					"trigger":         "auto",
					"compact_summary": "summary text",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("PostToolBatch via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(PostToolBatchInput)
			require.True(t, ok)
			require.Len(t, ev.ToolCalls, 3)
			assert.Equal(t, "Read", ev.ToolCalls[0].ToolName)
			assert.Equal(t, "tu_1", ev.ToolCalls[0].ToolUseID)
			assert.JSONEq(t, `{"file_path":"a.go"}`, string(ev.ToolCalls[0].ToolInput))
			assert.JSONEq(t, `"contents"`, string(ev.ToolCalls[0].ToolResponse))

			// Absent tool_response → empty.
			assert.Equal(t, "Grep", ev.ToolCalls[1].ToolName)
			assert.Empty(t, ev.ToolCalls[1].ToolResponse)

			// Explicit JSON null → preserved as "null", not conflated with absent.
			assert.Equal(t, "Bash", ev.ToolCalls[2].ToolName)
			assert.Equal(t, "null", string(ev.ToolCalls[2].ToolResponse))
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name": "PostToolBatch",
					"tool_calls": []interface{}{
						map[string]interface{}{
							"tool_name":     "Read",
							"tool_input":    map[string]interface{}{"file_path": "a.go"},
							"tool_use_id":   "tu_1",
							"tool_response": "contents",
						},
						map[string]interface{}{
							"tool_name":   "Grep",
							"tool_input":  map[string]interface{}{"pattern": "foo"},
							"tool_use_id": "tu_2",
						},
						map[string]interface{}{
							"tool_name":     "Bash",
							"tool_input":    map[string]interface{}{"command": "true"},
							"tool_use_id":   "tu_3",
							"tool_response": nil,
						},
					},
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("Setup via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(SetupInput)
			require.True(t, ok)
			assert.Equal(t, "init", ev.Trigger)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event": "Setup",
					"trigger":    "init",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("StopFailure via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(StopFailureInput)
			require.True(t, ok)
			assert.Equal(t, AssistantMessageErrorRateLimit, ev.Error)
			assert.Equal(t, "rate limited by upstream", ev.ErrorDetails)
			assert.Equal(t, "partial answer", ev.LastAssistantMessage)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name":        "StopFailure",
					"error":                  "rate_limit",
					"error_details":          "rate limited by upstream",
					"last_assistant_message": "partial answer",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("TaskCompleted via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(TaskCompletedInput)
			require.True(t, ok)
			assert.Equal(t, "task_42", ev.TaskID)
			assert.Equal(t, "ship the thing", ev.TaskSubject)
			assert.Equal(t, "longer description", ev.TaskDescription)
			assert.Equal(t, "alice", ev.TeammateName)
			assert.Equal(t, "platform", ev.TeamName)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event":       "TaskCompleted",
					"task_id":          "task_42",
					"task_subject":     "ship the thing",
					"task_description": "longer description",
					"teammate_name":    "alice",
					"team_name":        "platform",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("TaskCreated via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(TaskCreatedInput)
			require.True(t, ok)
			assert.Equal(t, "task_99", ev.TaskID)
			assert.Equal(t, "draft proposal", ev.TaskSubject)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name": "TaskCreated",
					"task_id":         "task_99",
					"task_subject":    "draft proposal",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("TeammateIdle via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(TeammateIdleInput)
			require.True(t, ok)
			assert.Equal(t, "bob", ev.TeammateName)
			assert.Equal(t, "platform", ev.TeamName)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event":    "TeammateIdle",
					"teammate_name": "bob",
					"team_name":     "platform",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("UserPromptExpansion via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(UserPromptExpansionInput)
			require.True(t, ok)
			assert.Equal(t, "slash_command", ev.ExpansionType)
			assert.Equal(t, "review", ev.CommandName)
			assert.Equal(t, "PR-30", ev.CommandArgs)
			assert.Equal(t, "user", ev.CommandSource)
			assert.Equal(t, "expanded prompt body", ev.Prompt)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name": "UserPromptExpansion",
					"expansion_type":  "slash_command",
					"command_name":    "review",
					"command_args":    "PR-30",
					"command_source":  "user",
					"prompt":          "expanded prompt body",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("WorktreeCreate via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(WorktreeCreateInput)
			require.True(t, ok)
			assert.Equal(t, "feature-x", ev.Name)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event": "WorktreeCreate",
					"name":       "feature-x",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("WorktreeRemove via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(WorktreeRemoveInput)
			require.True(t, ok)
			assert.Equal(t, "/repo/worktrees/feature-x", ev.WorktreePath)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name": "WorktreeRemove",
					"worktree_path":   "/repo/worktrees/feature-x",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})
}

func TestHandleHookCallback_RetryWatchPathsEvents(t *testing.T) {
	t.Run("CwdChanged via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(CwdChangedInput)
			require.True(t, ok)
			assert.Equal(t, "/repo/old", ev.OldCwd)
			assert.Equal(t, "/repo/new", ev.NewCwd)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event": "CwdChanged",
					"old_cwd":    "/repo/old",
					"new_cwd":    "/repo/new",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("FileChanged via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(FileChangedInput)
			require.True(t, ok)
			assert.Equal(t, "/repo/main.go", ev.FilePath)
			assert.Equal(t, "change", ev.Event)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name": "FileChanged",
					"file_path":       "/repo/main.go",
					"event":           "change",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("PermissionDenied via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(PermissionDeniedInput)
			require.True(t, ok)
			assert.Equal(t, "Bash", ev.ToolName)
			assert.JSONEq(t, `{"command":"rm -rf build"}`, string(ev.ToolInput))
			assert.Equal(t, "toolu_123", ev.ToolUseID)
			assert.Equal(t, "policy denied", ev.Reason)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event":  "PermissionDenied",
					"tool_name":   "Bash",
					"tool_input":  map[string]interface{}{"command": "rm -rf build"},
					"tool_use_id": "toolu_123",
					"reason":      "policy denied",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("Elicitation form via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(ElicitationInput)
			require.True(t, ok)
			assert.Equal(t, "payments", ev.MCPServerName)
			assert.Equal(t, "Need account details", ev.Message)
			assert.Equal(t, "form", ev.Mode)
			assert.Equal(t, "elicit_1", ev.ElicitationID)
			require.NotNil(t, ev.RequestedSchema)
			assert.Equal(t, "object", ev.RequestedSchema["type"])
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name":  "Elicitation",
					"mcp_server_name":  "payments",
					"message":          "Need account details",
					"mode":             "form",
					"elicitation_id":   "elicit_1",
					"requested_schema": map[string]interface{}{"type": "object"},
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("Elicitation url via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(ElicitationInput)
			require.True(t, ok)
			assert.Equal(t, "identity", ev.MCPServerName)
			assert.Equal(t, "Authorize access", ev.Message)
			assert.Equal(t, "url", ev.Mode)
			assert.Equal(t, "https://example.com/auth", ev.URL)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event":      "Elicitation",
					"mcp_server_name": "identity",
					"message":         "Authorize access",
					"mode":            "url",
					"url":             "https://example.com/auth",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("ElicitationResult form via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(ElicitationResultInput)
			require.True(t, ok)
			assert.Equal(t, "payments", ev.MCPServerName)
			assert.Equal(t, "elicit_1", ev.ElicitationID)
			assert.Equal(t, "form", ev.Mode)
			assert.Equal(t, "accept", ev.Action)
			require.NotNil(t, ev.Content)
			assert.Equal(t, "acct_123", ev.Content["account_id"])
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event":      "ElicitationResult",
					"mcp_server_name": "payments",
					"elicitation_id":  "elicit_1",
					"mode":            "form",
					"action":          "accept",
					"content":         map[string]interface{}{"account_id": "acct_123"},
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("ElicitationResult url via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(ElicitationResultInput)
			require.True(t, ok)
			assert.Equal(t, "identity", ev.MCPServerName)
			assert.Equal(t, "url", ev.Mode)
			assert.Equal(t, "decline", ev.Action)
			assert.Nil(t, ev.Content)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name": "ElicitationResult",
					"mcp_server_name": "identity",
					"mode":            "url",
					"action":          "decline",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})
}
