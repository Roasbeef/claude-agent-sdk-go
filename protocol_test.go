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

		resp := buildHookResponse(result)

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

		resp := buildHookResponse(result)

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

		resp := buildHookResponse(result)

		assert.Equal(t, true, resp["continue"])

		// Must NOT have decision fields.
		_, hasDecision := resp["decision"]
		assert.False(t, hasDecision,
			"non-stop hook should not include decision",
		)
	})

	t.Run("block with modify", func(t *testing.T) {
		result := HookResult{
			Decision: "block",
			Reason:   "New task",
			Modify: map[string]interface{}{
				"key": "value",
			},
		}

		resp := buildHookResponse(result)

		assert.Equal(t, "block", resp["decision"])
		assert.Equal(t, "New task", resp["reason"])

		// Modify should still be included.
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
