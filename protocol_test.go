package claudeagent

import (
	"context"
	"encoding/json"
	"errors"
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

func TestProtocolReinitializeBypassesGuard(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, transport.Connect(ctx))
	defer transport.Close()

	readerReady := make(chan struct{})
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
	<-readerReady

	// Answer every initialize request the CLI receives with a distinguishable
	// output_style so we can prove a fresh request was actually sent.
	go func() {
		decoder := json.NewDecoder(runner.StdinPipe)
		for {
			var req SDKControlRequest
			if err := decoder.Decode(&req); err != nil {
				return
			}
			resp := SDKControlResponse{
				Type: "control_response",
				Response: SDKControlResponseBody{
					Subtype:   "success",
					RequestID: req.RequestID,
					Response:  map[string]interface{}{"output_style": "fresh"},
				},
			}
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			runner.StdoutPipe.Write(data)
		}
	}()

	// Simulate an already-initialized session: Initialize would no-op, but
	// Reinitialize must still issue a fresh request and refresh the cache.
	protocol.initialized.Store(true)

	type result struct {
		resp *SDKControlInitializeResponse
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := protocol.Reinitialize(ctx)
		done <- result{resp, err}
	}()

	select {
	case r := <-done:
		require.NoError(t, r.err)
		require.NotNil(t, r.resp)
		assert.Equal(t, "fresh", r.resp.OutputStyle)
	case <-ctx.Done():
		t.Fatal("timeout waiting for Reinitialize to complete")
	}

	assert.Equal(t, "fresh", protocol.initResult().OutputStyle,
		"Reinitialize should refresh the cached initialize response")
}

func TestProtocolInitializeSupportedDialogKindsRequiresCallback(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	opts.SupportedDialogKinds = []string{"refusal_fallback_prompt"}
	// No OnUserDialog set — the pairing is invalid. The check fires before
	// any transport interaction, so an unconnected transport is fine.

	transport := NewSubprocessTransportWithRunner(runner, opts)
	protocol := NewProtocol(transport, opts)

	err := protocol.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WithOnUserDialog")
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
				WithToolAliases(map[string]string{
					"Bash": "mcp__workspace__bash",
				})(opts)
				WithExcludeDynamicSystemPromptSections(true)(opts)
			},
			expected: map[string]interface{}{
				"planModeInstructions":   "plan deliberately",
				"title":                  "session title",
				"skills":                 []interface{}{"go", "review"},
				"promptSuggestions":      false,
				"agentProgressSummaries": true,
				"forwardSubagentText":    false,
				"toolAliases":            map[string]interface{}{"Bash": "mcp__workspace__bash"},
				"excludeDynamicSections": true,
			},
		},
		{
			name: "empty tool aliases omitted",
			configure: func(opts *Options) {
				WithToolAliases(map[string]string{})(opts)
			},
			unexpected: []string{"toolAliases"},
		},
		{
			name:       "nil tool aliases omitted",
			configure:  func(opts *Options) {},
			unexpected: []string{"toolAliases"},
		},
		{
			name: "mcp server timeout wired through",
			configure: func(opts *Options) {
				timeout := 5000
				WithMCPServers(map[string]MCPServerConfig{
					"local": {
						Type:    "stdio",
						Command: "foo",
						Timeout: &timeout,
					},
				})(opts)
			},
			expected: map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"local": map[string]interface{}{
						"type":    "stdio",
						"command": "foo",
						"timeout": float64(5000),
					},
				},
			},
		},
		{
			name: "mcp server alwaysLoad wired through",
			configure: func(opts *Options) {
				alwaysLoad := true
				WithMCPServers(map[string]MCPServerConfig{
					"local": {
						Type:       "stdio",
						Command:    "foo",
						AlwaysLoad: &alwaysLoad,
					},
				})(opts)
			},
			expected: map[string]interface{}{
				"mcpServers": map[string]interface{}{
					"local": map[string]interface{}{
						"type":       "stdio",
						"command":    "foo",
						"alwaysLoad": true,
					},
				},
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
		{
			name: "sdk mcp server timeout wired through",
			configure: func(opts *Options) {
				timeout := 45000
				WithMcpServer("timed", CreateMcpServer(McpServerOptions{
					Name:    "timed",
					Timeout: &timeout,
				}))(opts)
			},
			expected: map[string]interface{}{
				"sdkMcpServers": []interface{}{"timed"},
				"sdkMcpServerConfigs": map[string]interface{}{
					"timed": map[string]interface{}{
						"timeout": float64(45000),
					},
				},
			},
		},
		{
			name: "sdk mcp server without timeout omits configs",
			configure: func(opts *Options) {
				WithMcpServer("plain", CreateMcpServer(McpServerOptions{
					Name: "plain",
				}))(opts)
			},
			expected: map[string]interface{}{
				"sdkMcpServers": []interface{}{"plain"},
			},
			unexpected: []string{"sdkMcpServerConfigs"},
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

func TestProtocolInitializeHookTimeout(t *testing.T) {
	callback := func(ctx context.Context, input HookInput) (HookResult, error) {
		return HookResult{Continue: true}, nil
	}

	tests := []struct {
		name    string
		configs []HookConfig
		assert  func(*testing.T, SDKControlRequest)
	}{
		{
			name: "timeout set",
			configs: []HookConfig{
				{
					Matcher:  "*",
					Timeout:  30,
					Callback: callback,
				},
			},
			assert: func(t *testing.T, initReq SDKControlRequest) {
				matchers := initReq.Request.Hooks[string(HookTypePreToolUse)]
				require.Len(t, matchers, 1)
				assert.Equal(t, "*", matchers[0].Matcher)
				assert.Equal(t, 30, matchers[0].Timeout)

				got := marshalHookMatcher(t, matchers[0])
				assert.Equal(t, float64(30), got["timeout"])
			},
		},
		{
			name: "timeout zero omitted",
			configs: []HookConfig{
				{
					Matcher:  "*",
					Callback: callback,
				},
			},
			assert: func(t *testing.T, initReq SDKControlRequest) {
				matchers := initReq.Request.Hooks[string(HookTypePreToolUse)]
				require.Len(t, matchers, 1)
				assert.Zero(t, matchers[0].Timeout)

				got := marshalHookMatcher(t, matchers[0])
				assert.NotContains(t, got, "timeout")
			},
		},
		{
			name: "multiple matchers mixed",
			configs: []HookConfig{
				{
					Matcher:  "Bash",
					Timeout:  15,
					Callback: callback,
				},
				{
					Matcher:  "Read",
					Callback: callback,
				},
			},
			assert: func(t *testing.T, initReq SDKControlRequest) {
				matchers := initReq.Request.Hooks[string(HookTypePreToolUse)]
				require.Len(t, matchers, 2)
				assert.Equal(t, "Bash", matchers[0].Matcher)
				assert.Equal(t, 15, matchers[0].Timeout)
				assert.Equal(t, "Read", matchers[1].Matcher)
				assert.Zero(t, matchers[1].Timeout)

				withTimeout := marshalHookMatcher(t, matchers[0])
				withoutTimeout := marshalHookMatcher(t, matchers[1])
				assert.Equal(t, float64(15), withTimeout["timeout"])
				assert.NotContains(t, withoutTimeout, "timeout")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := NewMockSubprocessRunner()
			opts := NewOptions()
			opts.Hooks = map[HookType][]HookConfig{
				HookTypePreToolUse: tt.configs,
			}

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

			tt.assert(t, initReq)
		})
	}
}

func marshalHookMatcher(t *testing.T, matcher SDKHookCallbackMatcher) map[string]interface{} {
	t.Helper()

	data, err := json.Marshal(matcher)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	return got
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

func TestProtocolHandlePermissionRequestClassification(t *testing.T) {
	tests := []struct {
		name       string
		result     PermissionResult
		expected   map[string]interface{}
		unexpected []string
	}{
		{
			name:   "allow without classification",
			result: PermissionAllow{},
			expected: map[string]interface{}{
				"allowed": true,
			},
			unexpected: []string{"decisionClassification"},
		},
		{
			name: "allow with classification",
			result: PermissionAllow{
				Classification: PermissionClassificationUserPermanent,
			},
			expected: map[string]interface{}{
				"allowed":                true,
				"decisionClassification": "user_permanent",
			},
		},
		{
			name:   "deny without classification",
			result: PermissionDeny{Reason: "no"},
			expected: map[string]interface{}{
				"allowed": false,
				"reason":  "no",
			},
			unexpected: []string{"decisionClassification"},
		},
		{
			name: "deny with classification",
			result: PermissionDeny{
				Reason:         "no",
				Classification: PermissionClassificationUserReject,
			},
			expected: map[string]interface{}{
				"allowed":                false,
				"reason":                 "no",
				"decisionClassification": "user_reject",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.CanUseTool = func(ctx context.Context, req ToolPermissionRequest) PermissionResult {
				return tt.result
			}
			protocol := NewProtocol(nil, opts)

			resp := protocol.handlePermissionRequest(context.Background(), ControlRequest{
				Type:      "control",
				Subtype:   "can_use_tool",
				RequestID: "req_1",
				Payload: map[string]interface{}{
					"tool_name":   "fetch_quote",
					"tool_use_id": "tool_1",
					"input":       map[string]interface{}{},
				},
			})

			require.Equal(t, "success", resp.Response.Subtype)
			respData := resp.Response.Response
			for key, want := range tt.expected {
				assert.Equal(t, want, respData[key])
			}
			for _, key := range tt.unexpected {
				assert.NotContains(t, respData, key)
			}
		})
	}
}

func TestProtocolHandlePermissionRequestRequiresUserInteraction(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
		want    bool
	}{
		{
			name: "flag true is parsed into context",
			payload: map[string]interface{}{
				"tool_name":                 "AskUserQuestion",
				"tool_use_id":               "tool_1",
				"input":                     map[string]interface{}{},
				"requires_user_interaction": true,
			},
			want: true,
		},
		{
			name: "absent flag defaults to false",
			payload: map[string]interface{}{
				"tool_name":   "fetch_quote",
				"tool_use_id": "tool_1",
				"input":       map[string]interface{}{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured PermissionContext
			opts := NewOptions()
			opts.CanUseTool = func(ctx context.Context, req ToolPermissionRequest) PermissionResult {
				captured = req.Context
				return PermissionAllow{}
			}
			protocol := NewProtocol(nil, opts)

			resp := protocol.handlePermissionRequest(context.Background(), ControlRequest{
				Type:      "control",
				Subtype:   "can_use_tool",
				RequestID: "req_1",
				Payload:   tt.payload,
			})

			require.Equal(t, "success", resp.Response.Subtype)
			assert.Equal(t, tt.want, captured.RequiresUserInteraction)
		})
	}
}

func TestProtocolHandlePermissionRequestAskRule(t *testing.T) {
	t.Run("fields parsed into context", func(t *testing.T) {
		var captured PermissionContext
		opts := NewOptions()
		opts.CanUseTool = func(ctx context.Context, req ToolPermissionRequest) PermissionResult {
			captured = req.Context
			return PermissionAllow{}
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handlePermissionRequest(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "can_use_tool",
			RequestID: "req_1",
			Payload: map[string]interface{}{
				"tool_name":                  "Bash",
				"tool_use_id":                "tool_1",
				"input":                      map[string]interface{}{},
				"suppress_always_allow_rule": true,
				"matched_ask_rule": map[string]interface{}{
					"source":       "settings",
					"tool_name":    "Bash",
					"rule_content": "Bash(rm:*)",
				},
			},
		})

		require.Equal(t, "success", resp.Response.Subtype)
		assert.True(t, captured.SuppressAlwaysAllowRule)
		require.NotNil(t, captured.MatchedAskRule)
		assert.Equal(t, "settings", captured.MatchedAskRule.Source)
		assert.Equal(t, "Bash", captured.MatchedAskRule.ToolName)
		assert.Equal(t, "Bash(rm:*)", captured.MatchedAskRule.RuleContent)
	})

	t.Run("absent fields leave zero values", func(t *testing.T) {
		var captured PermissionContext
		opts := NewOptions()
		opts.CanUseTool = func(ctx context.Context, req ToolPermissionRequest) PermissionResult {
			captured = req.Context
			return PermissionAllow{}
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handlePermissionRequest(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "can_use_tool",
			RequestID: "req_1",
			Payload: map[string]interface{}{
				"tool_name":   "fetch_quote",
				"tool_use_id": "tool_1",
				"input":       map[string]interface{}{},
			},
		})

		require.Equal(t, "success", resp.Response.Subtype)
		assert.False(t, captured.SuppressAlwaysAllowRule)
		assert.Nil(t, captured.MatchedAskRule)
	})
}

func TestProtocolHandleSDKPermissionRequestClassification(t *testing.T) {
	tests := []struct {
		name       string
		result     PermissionResult
		expected   map[string]interface{}
		unexpected []string
	}{
		{
			name:   "allow without classification",
			result: PermissionAllow{},
			expected: map[string]interface{}{
				"behavior":  "allow",
				"toolUseID": "tool_1",
			},
			unexpected: []string{"decisionClassification"},
		},
		{
			name: "allow with classification",
			result: PermissionAllow{
				Classification: PermissionClassificationUserPermanent,
			},
			expected: map[string]interface{}{
				"behavior":               "allow",
				"decisionClassification": "user_permanent",
				"toolUseID":              "tool_1",
			},
		},
		{
			name:   "deny without classification",
			result: PermissionDeny{Reason: "no"},
			expected: map[string]interface{}{
				"behavior":  "deny",
				"message":   "no",
				"toolUseID": "tool_1",
			},
			unexpected: []string{"decisionClassification"},
		},
		{
			name: "deny with classification",
			result: PermissionDeny{
				Reason:         "no",
				Classification: PermissionClassificationUserReject,
			},
			expected: map[string]interface{}{
				"behavior":               "deny",
				"message":                "no",
				"decisionClassification": "user_reject",
				"toolUseID":              "tool_1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.CanUseTool = func(ctx context.Context, req ToolPermissionRequest) PermissionResult {
				return tt.result
			}
			protocol := NewProtocol(nil, opts)

			resp := protocol.handleSDKPermissionRequest(context.Background(), SDKControlRequest{
				Type:      "control_request",
				RequestID: "req_1",
				Request: SDKControlRequestBody{
					Subtype:   "can_use_tool",
					ToolName:  "fetch_quote",
					ToolUseID: "tool_1",
					Input:     map[string]interface{}{},
				},
			})

			require.Equal(t, "success", resp.Response.Subtype)
			respData := resp.Response.Response
			for key, want := range tt.expected {
				assert.Equal(t, want, respData[key])
			}
			for _, key := range tt.unexpected {
				assert.NotContains(t, respData, key)
			}
		})
	}
}

func TestProtocolHandleElicitationRequest(t *testing.T) {
	basePayload := map[string]interface{}{
		"mcp_server_name":  "auth-server",
		"message":          "Enter credentials",
		"mode":             "form",
		"url":              "https://example.com/auth",
		"elicitation_id":   "elicit_1",
		"requested_schema": map[string]interface{}{"type": "object"},
		"title":            "Sign in",
		"display_name":     "Auth Server",
		"description":      "Authentication required",
	}

	tests := []struct {
		name      string
		callback  OnElicitationFunc
		want      map[string]interface{}
		noContent bool
	}{
		{
			name: "accept with content",
			callback: func(ctx context.Context, req ElicitationRequest) (ElicitationResult, error) {
				return ElicitationResult{
					Action:  ElicitationActionAccept,
					Content: map[string]interface{}{"name": "alice"},
				}, nil
			},
			want: map[string]interface{}{
				"action":  "accept",
				"content": map[string]interface{}{"name": "alice"},
			},
		},
		{
			name: "decline",
			callback: func(ctx context.Context, req ElicitationRequest) (ElicitationResult, error) {
				return ElicitationResult{Action: ElicitationActionDecline}, nil
			},
			want:      map[string]interface{}{"action": "decline"},
			noContent: true,
		},
		{
			name: "cancel",
			callback: func(ctx context.Context, req ElicitationRequest) (ElicitationResult, error) {
				return ElicitationResult{Action: ElicitationActionCancel}, nil
			},
			want:      map[string]interface{}{"action": "cancel"},
			noContent: true,
		},
		{
			name: "callback error cancels",
			callback: func(ctx context.Context, req ElicitationRequest) (ElicitationResult, error) {
				return ElicitationResult{Action: ElicitationActionAccept}, errors.New("failed")
			},
			want:      map[string]interface{}{"action": "cancel"},
			noContent: true,
		},
		{
			name:      "nil callback declines",
			want:      map[string]interface{}{"action": "decline"},
			noContent: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.OnElicitation = tt.callback
			protocol := NewProtocol(nil, opts)

			resp := protocol.handleElicitationRequest(context.Background(), ControlRequest{
				Type:      "control",
				Subtype:   "elicitation",
				RequestID: "req_elicit",
				Payload:   basePayload,
			})

			assert.Equal(t, "control_response", resp.Type)
			assert.Equal(t, "success", resp.Response.Subtype)
			assert.Equal(t, "req_elicit", resp.Response.RequestID)
			assert.Equal(t, tt.want, resp.Response.Response)
			if tt.noContent {
				_, hasContent := resp.Response.Response["content"]
				assert.False(t, hasContent)
			}
		})
	}

	t.Run("request fields propagated", func(t *testing.T) {
		opts := NewOptions()
		var got ElicitationRequest
		opts.OnElicitation = func(ctx context.Context, req ElicitationRequest) (ElicitationResult, error) {
			got = req
			return ElicitationResult{Action: ElicitationActionDecline}, nil
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleElicitationRequest(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "elicitation",
			RequestID: "req_fields",
			Payload:   basePayload,
		})

		assert.Equal(t, "decline", resp.Response.Response["action"])
		assert.Equal(t, "auth-server", got.ServerName)
		assert.Equal(t, "Enter credentials", got.Message)
		assert.Equal(t, "form", got.Mode)
		assert.Equal(t, "https://example.com/auth", got.URL)
		assert.Equal(t, "elicit_1", got.ElicitationID)
		assert.Equal(t, map[string]interface{}{"type": "object"}, got.RequestedSchema)
		assert.Equal(t, "Sign in", got.Title)
		assert.Equal(t, "Auth Server", got.DisplayName)
		assert.Equal(t, "Authentication required", got.Description)
	})
}

func TestProtocolHandleUserDialogRequest(t *testing.T) {
	basePayload := map[string]interface{}{
		"dialog_kind": "approve_edit",
		"payload":     map[string]interface{}{"path": "/tmp/x", "lines": 12},
		"tool_use_id": "tu_42",
	}

	t.Run("invokes callback and returns completed", func(t *testing.T) {
		opts := NewOptions()
		var got UserDialogRequest
		opts.OnUserDialog = func(ctx context.Context, req UserDialogRequest) (UserDialogResult, error) {
			got = req
			return UserDialogResult{
				Behavior: UserDialogBehaviorCompleted,
				Result:   "yes",
			}, nil
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleUserDialogRequest(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "request_user_dialog",
			RequestID: "req_ud",
			Payload:   basePayload,
		})

		assert.Equal(t, "control_response", resp.Type)
		assert.Equal(t, "success", resp.Response.Subtype)
		assert.Equal(t, "req_ud", resp.Response.RequestID)
		assert.Equal(t, map[string]interface{}{
			"behavior": "completed",
			"result":   "yes",
		}, resp.Response.Response)

		assert.Equal(t, "approve_edit", got.DialogKind)
		assert.Equal(t, map[string]interface{}{"path": "/tmp/x", "lines": 12}, got.Payload)
		assert.Equal(t, "tu_42", got.ToolUseID)
	})

	t.Run("nil callback answers cancelled", func(t *testing.T) {
		opts := NewOptions()
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleUserDialogRequest(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "request_user_dialog",
			RequestID: "req_nil",
			Payload:   basePayload,
		})

		assert.Equal(t, "success", resp.Response.Subtype)
		assert.Equal(t, map[string]interface{}{"behavior": "cancelled"}, resp.Response.Response)
		_, hasResult := resp.Response.Response["result"]
		assert.False(t, hasResult)
	})

	t.Run("callback cancels", func(t *testing.T) {
		opts := NewOptions()
		opts.OnUserDialog = func(ctx context.Context, req UserDialogRequest) (UserDialogResult, error) {
			return UserDialogResult{Behavior: UserDialogBehaviorCancelled}, nil
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleUserDialogRequest(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "request_user_dialog",
			RequestID: "req_cancel",
			Payload:   basePayload,
		})

		assert.Equal(t, "success", resp.Response.Subtype)
		assert.Equal(t, map[string]interface{}{"behavior": "cancelled"}, resp.Response.Response)
		_, hasResult := resp.Response.Response["result"]
		assert.False(t, hasResult)
	})

	t.Run("callback error answers cancelled", func(t *testing.T) {
		opts := NewOptions()
		opts.OnUserDialog = func(ctx context.Context, req UserDialogRequest) (UserDialogResult, error) {
			return UserDialogResult{
				Behavior: UserDialogBehaviorCompleted,
				Result:   "ignored",
			}, errors.New("renderer crashed")
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleUserDialogRequest(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "request_user_dialog",
			RequestID: "req_err",
			Payload:   basePayload,
		})

		assert.Equal(t, "success", resp.Response.Subtype)
		assert.Equal(t, map[string]interface{}{"behavior": "cancelled"}, resp.Response.Response)
	})
}

func TestProtocolHandleHostAuthTokenRefresh(t *testing.T) {
	t.Run("callback returns token", func(t *testing.T) {
		opts := NewOptions()
		opts.GetHostAuthToken = func(ctx context.Context) (string, error) {
			return "abc", nil
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleHostAuthTokenRefresh(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "host_auth_token_refresh",
			RequestID: "req_host_auth",
		})

		assert.Equal(t, "control_response", resp.Type)
		assert.Equal(t, "success", resp.Response.Subtype)
		assert.Equal(t, "req_host_auth", resp.Response.RequestID)
		assert.Equal(t, map[string]interface{}{"authToken": "abc"}, resp.Response.Response)
	})

	t.Run("callback absent errors", func(t *testing.T) {
		opts := NewOptions()
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleHostAuthTokenRefresh(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "host_auth_token_refresh",
			RequestID: "req_host_auth",
		})

		assert.Equal(t, "control_response", resp.Type)
		assert.Equal(t, "error", resp.Response.Subtype)
		assert.Equal(t, "req_host_auth", resp.Response.RequestID)
		assert.Contains(t, resp.Response.Error, "GetHostAuthToken")
	})

	t.Run("callback returns error", func(t *testing.T) {
		opts := NewOptions()
		opts.GetHostAuthToken = func(ctx context.Context) (string, error) {
			return "", errors.New("refresh failed")
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleHostAuthTokenRefresh(context.Background(), ControlRequest{
			Type:      "control",
			Subtype:   "host_auth_token_refresh",
			RequestID: "req_host_auth",
		})

		assert.Equal(t, "control_response", resp.Type)
		assert.Equal(t, "error", resp.Response.Subtype)
		assert.Equal(t, "req_host_auth", resp.Response.RequestID)
		assert.Equal(t, "refresh failed", resp.Response.Error)
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
	t.Run("initialize instructions", func(t *testing.T) {
		server := CreateMcpServer(McpServerOptions{
			Name:         "docs",
			Version:      "2.0.0",
			Instructions: "use this server for docs",
		})
		opts := NewOptions()
		opts.SDKMcpServers = map[string]*McpServer{
			"docs": server,
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleSDKMCPMessage(context.Background(), SDKControlRequest{
			Type:      "control_request",
			RequestID: "sdk_mcp_init",
			Request: SDKControlRequestBody{
				Subtype:    "mcp_message",
				ServerName: "docs",
				Message: map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      "msg_init",
					"method":  "initialize",
				},
			},
		})

		require.Equal(t, "success", resp.Response.Subtype)
		mcpResponse, ok := resp.Response.Response["mcp_response"].(map[string]interface{})
		require.True(t, ok, "mcp_response should be a map")
		result, ok := mcpResponse["result"].(map[string]interface{})
		require.True(t, ok, "result should be a map")
		assert.Equal(t, "use this server for docs", result["instructions"])
	})

	t.Run("initialize omits empty instructions", func(t *testing.T) {
		server := CreateMcpServer(McpServerOptions{Name: "docs"})
		opts := NewOptions()
		opts.SDKMcpServers = map[string]*McpServer{
			"docs": server,
		}
		protocol := NewProtocol(nil, opts)

		resp := protocol.handleSDKMCPMessage(context.Background(), SDKControlRequest{
			Type:      "control_request",
			RequestID: "sdk_mcp_init",
			Request: SDKControlRequestBody{
				Subtype:    "mcp_message",
				ServerName: "docs",
				Message: map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      "msg_init",
					"method":  "initialize",
				},
			},
		})

		require.Equal(t, "success", resp.Response.Subtype)
		mcpResponse, ok := resp.Response.Response["mcp_response"].(map[string]interface{})
		require.True(t, ok, "mcp_response should be a map")
		result, ok := mcpResponse["result"].(map[string]interface{})
		require.True(t, ok, "result should be a map")
		assert.NotContains(t, result, "instructions")
	})

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
		require.Len(t, stopInput.BackgroundTasks, 2)
		assert.Equal(t, BackgroundTaskSummary{
			ID:          "task_shell",
			Type:        "shell",
			Status:      "running",
			Description: "running make test",
			Command:     "make test",
		}, stopInput.BackgroundTasks[0])
		assert.Equal(t, BackgroundTaskSummary{
			ID:          "task_agent",
			Type:        "subagent",
			Status:      "pending",
			Description: "planning followup",
			AgentType:   "planner",
		}, stopInput.BackgroundTasks[1])
		require.Len(t, stopInput.SessionCrons, 2)
		assert.Equal(t, SessionCronSummary{
			ID:        "cron_weekday",
			Schedule:  "0 9 * * 1-5",
			Recurring: true,
			Prompt:    "weekday standup",
		}, stopInput.SessionCrons[0])
		assert.Equal(t, SessionCronSummary{
			ID:        "cron_wakeup",
			Schedule:  "15 14 1 6 *",
			Recurring: false,
			Prompt:    "resume investigation",
		}, stopInput.SessionCrons[1])
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
				"background_tasks": []interface{}{
					map[string]interface{}{
						"id":          "task_shell",
						"type":        "shell",
						"status":      "running",
						"description": "running make test",
						"command":     "make test",
					},
					map[string]interface{}{
						"id":          "task_agent",
						"type":        "subagent",
						"status":      "pending",
						"description": "planning followup",
						"agent_type":  "planner",
					},
				},
				"session_crons": []interface{}{
					map[string]interface{}{
						"id":        "cron_weekday",
						"schedule":  "0 9 * * 1-5",
						"recurring": true,
						"prompt":    "weekday standup",
					},
					map[string]interface{}{
						"id":        "cron_wakeup",
						"schedule":  "15 14 1 6 *",
						"recurring": false,
						"prompt":    "resume investigation",
					},
				},
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
		require.Len(t, subagentStopInput.BackgroundTasks, 2)
		assert.Equal(t, BackgroundTaskSummary{
			ID:          "task_shell",
			Type:        "shell",
			Status:      "running",
			Description: "running make test",
			Command:     "make test",
		}, subagentStopInput.BackgroundTasks[0])
		assert.Equal(t, BackgroundTaskSummary{
			ID:          "task_agent",
			Type:        "subagent",
			Status:      "pending",
			Description: "planning followup",
			AgentType:   "builder",
		}, subagentStopInput.BackgroundTasks[1])
		require.Len(t, subagentStopInput.SessionCrons, 2)
		assert.Equal(t, SessionCronSummary{
			ID:        "cron_weekday",
			Schedule:  "0 9 * * 1-5",
			Recurring: true,
			Prompt:    "weekday standup",
		}, subagentStopInput.SessionCrons[0])
		assert.Equal(t, SessionCronSummary{
			ID:        "cron_wakeup",
			Schedule:  "15 14 1 6 *",
			Recurring: false,
			Prompt:    "resume investigation",
		}, subagentStopInput.SessionCrons[1])
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
				"background_tasks": []interface{}{
					map[string]interface{}{
						"id":          "task_shell",
						"type":        "shell",
						"status":      "running",
						"description": "running make test",
						"command":     "make test",
					},
					map[string]interface{}{
						"id":          "task_agent",
						"type":        "subagent",
						"status":      "pending",
						"description": "planning followup",
						"agent_type":  "builder",
					},
				},
				"session_crons": []interface{}{
					map[string]interface{}{
						"id":        "cron_weekday",
						"schedule":  "0 9 * * 1-5",
						"recurring": true,
						"prompt":    "weekday standup",
					},
					map[string]interface{}{
						"id":        "cron_wakeup",
						"schedule":  "15 14 1 6 *",
						"recurring": false,
						"prompt":    "resume investigation",
					},
				},
			},
		},
	})

	assert.Equal(t, "success", resp.Response.Subtype)
	assert.Equal(t, "sdk_subagent_stop", resp.Response.RequestID)
}

func TestHandleHookCallback_StopInputBackgroundTasksAbsent(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["hook_stop_absent"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		stopInput, ok := input.(StopInput)
		require.True(t, ok)
		assert.Nil(t, stopInput.BackgroundTasks)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleHookCallback(context.Background(), ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_stop_absent",
		Payload: map[string]interface{}{
			"callback_id": "hook_stop_absent",
			"input": map[string]interface{}{
				"hook_event":       "Stop",
				"stop_hook_active": false,
			},
		},
	})

	assert.Equal(t, "success", resp.Response.Subtype)
	assert.Equal(t, "req_stop_absent", resp.Response.RequestID)
}

func TestHandleHookCallback_StopInputSessionCronsAbsent(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["hook_stop_absent"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		stopInput, ok := input.(StopInput)
		require.True(t, ok)
		assert.Nil(t, stopInput.SessionCrons)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleHookCallback(context.Background(), ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_stop_absent",
		Payload: map[string]interface{}{
			"callback_id": "hook_stop_absent",
			"input": map[string]interface{}{
				"hook_event":       "Stop",
				"stop_hook_active": false,
			},
		},
	})

	assert.Equal(t, "success", resp.Response.Subtype)
	assert.Equal(t, "req_stop_absent", resp.Response.RequestID)
}

func TestStopInputBackgroundTasksJSONRoundTrip(t *testing.T) {
	input := StopInput{
		BaseHookInput: BaseHookInput{
			SessionID: "session_123",
		},
		StopHookActive:       true,
		LastAssistantMessage: "done",
		BackgroundTasks: []BackgroundTaskSummary{
			{
				ID:          "task_shell",
				Type:        "shell",
				Status:      "running",
				Description: "running make test",
				Command:     "make test",
			},
			{
				ID:          "task_agent",
				Type:        "subagent",
				Status:      "pending",
				Description: "planning followup",
				AgentType:   "planner",
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"background_tasks"`)
	assert.NotContains(t, string(data), `"agent_type":""`)
	assert.NotContains(t, string(data), `"command":""`)

	var got StopInput
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, input, got)
}

func TestStopInputSessionCronsJSONRoundTrip(t *testing.T) {
	input := StopInput{
		BaseHookInput: BaseHookInput{
			SessionID: "session_123",
		},
		StopHookActive:       true,
		LastAssistantMessage: "done",
		SessionCrons: []SessionCronSummary{
			{
				ID:        "cron_weekday",
				Schedule:  "0 9 * * 1-5",
				Recurring: true,
				Prompt:    "weekday standup",
			},
			{
				ID:        "cron_wakeup",
				Schedule:  "15 14 1 6 *",
				Recurring: false,
				Prompt:    "resume investigation",
			},
		},
	}

	data, err := json.Marshal(input)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"session_crons"`)

	var got StopInput
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, input, got)
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

	t.Run("PostToolUse without UpdatedToolOutput falls through to legacy modify", func(t *testing.T) {
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

// TestBuildHookResponse_PostToolUseUpdatedToolOutput verifies that
// HookResult.UpdatedToolOutput auto-translates into
// hookSpecificOutput.updatedToolOutput for PostToolUse hooks.
func TestBuildHookResponse_PostToolUseUpdatedToolOutput(t *testing.T) {
	t.Run("string output translates", func(t *testing.T) {
		result := HookResult{
			Continue:          true,
			UpdatedToolOutput: "rewritten output",
		}

		resp := buildHookResponse("PostToolUse", result)

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok, "PostToolUse should emit hookSpecificOutput")
		assert.Equal(t, "PostToolUse", hso["hookEventName"])
		assert.Equal(t, "rewritten output", hso["updatedToolOutput"])
		_, hasModify := resp["modify"]
		assert.False(t, hasModify, "modify key should not be present")
	})

	t.Run("structured output translates", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			UpdatedToolOutput: map[string]interface{}{
				"status": "ok",
				"lines":  3,
			},
		}

		resp := buildHookResponse("PostToolUse", result)

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		out, ok := hso["updatedToolOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ok", out["status"])
		assert.Equal(t, 3, out["lines"])
	})

	t.Run("nil UpdatedToolOutput falls through to legacy modify", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Modify:   map[string]interface{}{"key": "value"},
		}

		resp := buildHookResponse("PostToolUse", result)

		modify, ok := resp["modify"].(map[string]interface{})
		require.True(t, ok, "should fall through to legacy modify when UpdatedToolOutput is nil")
		assert.Equal(t, "value", modify["key"])
		_, hasHSO := resp["hookSpecificOutput"]
		assert.False(t, hasHSO)
	})

	t.Run("UpdatedToolOutput wins over Modify", func(t *testing.T) {
		result := HookResult{
			Continue:          true,
			UpdatedToolOutput: "winner",
			Modify:            map[string]interface{}{"loser": true},
		}

		resp := buildHookResponse("PostToolUse", result)

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "winner", hso["updatedToolOutput"])
		_, hasModify := resp["modify"]
		assert.False(t, hasModify, "Modify should be ignored when UpdatedToolOutput is set")
	})
}

// TestBuildHookResponse_TerminalSequence verifies that
// HookResult.TerminalSequence is emitted on the wire envelope for both
// Stop and non-Stop hook returns, that the empty string elides the
// field, and that it composes with HookSpecificOutput and WatchPaths
// without interference.
func TestBuildHookResponse_TerminalSequence(t *testing.T) {
	t.Run("emitted on non-Stop hook", func(t *testing.T) {
		resp := buildHookResponse("PreToolUse", HookResult{
			Continue:         true,
			TerminalSequence: "\x1b]9;done\x07",
		})

		assert.Equal(t, true, resp["continue"])
		assert.Equal(t, "\x1b]9;done\x07", resp["terminalSequence"])
	})

	t.Run("emitted on Stop hook alongside decision", func(t *testing.T) {
		resp := buildHookResponse("Stop", HookResult{
			Decision:         "block",
			Reason:           "continue work",
			TerminalSequence: "\x1b]777;notify;hi\x07",
		})

		assert.Equal(t, "block", resp["decision"])
		assert.Equal(t, "continue work", resp["reason"])
		assert.Equal(t, "\x1b]777;notify;hi\x07", resp["terminalSequence"])
		_, hasContinue := resp["continue"]
		assert.False(t, hasContinue, "Stop path must not emit continue")
	})

	t.Run("empty string elides field", func(t *testing.T) {
		resp := buildHookResponse("PreToolUse", HookResult{
			Continue: true,
		})

		_, has := resp["terminalSequence"]
		assert.False(t, has)
	})

	t.Run("composes with HookSpecificOutput", func(t *testing.T) {
		resp := buildHookResponse("PreToolUse", HookResult{
			Continue:         true,
			TerminalSequence: "\x07",
			HookSpecificOutput: map[string]interface{}{
				"hookEventName":      "PreToolUse",
				"permissionDecision": "ask",
			},
		})

		assert.Equal(t, "\x07", resp["terminalSequence"])
		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ask", hso["permissionDecision"])
	})

	t.Run("composes with WatchPaths", func(t *testing.T) {
		resp := buildHookResponse("SessionStart", HookResult{
			Continue:         true,
			TerminalSequence: "\x1b]9;watching\x07",
			WatchPaths:       []string{"/tmp/project"},
		})

		assert.Equal(t, "\x1b]9;watching\x07", resp["terminalSequence"])
		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, []string{"/tmp/project"}, hso["watchPaths"])
	})

	t.Run("HookJSONOutput round-trip", func(t *testing.T) {
		out := HookJSONOutput{
			Continue:         true,
			TerminalSequence: "\x1b]9;round-trip\x07",
		}
		data, err := json.Marshal(out)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"terminalSequence":"\u001b]9;round-trip\u0007"`)

		var back HookJSONOutput
		require.NoError(t, json.Unmarshal(data, &back))
		assert.Equal(t, "\x1b]9;round-trip\x07", back.TerminalSequence)

		emptyData, err := json.Marshal(HookJSONOutput{Continue: true})
		require.NoError(t, err)
		assert.NotContains(t, string(emptyData), "terminalSequence")
	})
}

// TestBuildHookResponse_SuppressOriginalPrompt verifies that
// HookResult.SuppressOriginalPrompt is emitted under hookSpecificOutput
// only for UserPromptSubmit hooks, composes with an explicit
// HookSpecificOutput map without clobbering existing keys, distinguishes
// nil from a pointer-to-false, and is ignored on other hook types.
func TestBuildHookResponse_SuppressOriginalPrompt(t *testing.T) {
	t.Run("emitted on UserPromptSubmit when true", func(t *testing.T) {
		v := true
		resp := buildHookResponse("UserPromptSubmit", HookResult{
			Continue:               true,
			SuppressOriginalPrompt: &v,
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "UserPromptSubmit", hso["hookEventName"])
		assert.Equal(t, true, hso["suppressOriginalPrompt"])
	})

	t.Run("explicit false is emitted", func(t *testing.T) {
		v := false
		resp := buildHookResponse("UserPromptSubmit", HookResult{
			Continue:               true,
			SuppressOriginalPrompt: &v,
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, false, hso["suppressOriginalPrompt"])
	})

	t.Run("nil elides field", func(t *testing.T) {
		resp := buildHookResponse("UserPromptSubmit", HookResult{
			Continue: true,
		})

		_, has := resp["hookSpecificOutput"]
		assert.False(t, has)
	})

	t.Run("composes with explicit HookSpecificOutput", func(t *testing.T) {
		v := true
		resp := buildHookResponse("UserPromptSubmit", HookResult{
			Continue:               true,
			SuppressOriginalPrompt: &v,
			HookSpecificOutput: map[string]interface{}{
				"hookEventName":     "UserPromptSubmit",
				"additionalContext": "redacted",
			},
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "UserPromptSubmit", hso["hookEventName"])
		assert.Equal(t, "redacted", hso["additionalContext"])
		assert.Equal(t, true, hso["suppressOriginalPrompt"])
	})

	t.Run("ignored on non-UserPromptSubmit hooks", func(t *testing.T) {
		v := true
		resp := buildHookResponse("PreToolUse", HookResult{
			Continue:               true,
			SuppressOriginalPrompt: &v,
		})

		_, has := resp["hookSpecificOutput"]
		assert.False(t, has,
			"suppressOriginalPrompt must not leak onto non-UserPromptSubmit hookSpecificOutput")
	})
}

func TestBuildHookResponse_Stop_AdditionalContext(t *testing.T) {
	resp := buildHookResponse("Stop", HookResult{
		Continue:          true,
		AdditionalContext: "context",
	})

	hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Stop", hso["hookEventName"])
	assert.Equal(t, "context", hso["additionalContext"])
}

func TestBuildHookResponse_Stop_AdditionalContextEmpty(t *testing.T) {
	resp := buildHookResponse("Stop", HookResult{
		Continue:          true,
		AdditionalContext: "",
	})

	_, has := resp["hookSpecificOutput"]
	assert.False(t, has,
		"empty AdditionalContext must not emit hookSpecificOutput")
}

func TestBuildHookResponse_SubagentStop_AdditionalContext(t *testing.T) {
	resp := buildHookResponse("SubagentStop", HookResult{
		Continue:          true,
		AdditionalContext: "context",
	})

	hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "SubagentStop", hso["hookEventName"])
	assert.Equal(t, "context", hso["additionalContext"])
}

func TestBuildHookResponse_SubagentStop_AdditionalContextEmpty(t *testing.T) {
	resp := buildHookResponse("SubagentStop", HookResult{
		Continue:          true,
		AdditionalContext: "",
	})

	_, has := resp["hookSpecificOutput"]
	assert.False(t, has,
		"empty AdditionalContext must not emit hookSpecificOutput")
}

func TestBuildHookResponse_AdditionalContext_DroppedOnUnsupportedHook(t *testing.T) {
	resp := buildHookResponse("PreCompact", HookResult{
		Continue:          true,
		AdditionalContext: "context",
	})

	_, has := resp["hookSpecificOutput"]
	assert.False(t, has,
		"additionalContext must not leak onto hooks whose envelope does not accept it")
}

func TestBuildHookResponse_AdditionalContext_ComposesWithHookSpecificOutput(t *testing.T) {
	resp := buildHookResponse("Stop", HookResult{
		Continue:          true,
		AdditionalContext: "from-typed-field",
		HookSpecificOutput: map[string]interface{}{
			"hookEventName": "Stop",
			"customKey":     "preserved",
		},
	})

	hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Stop", hso["hookEventName"])
	assert.Equal(t, "from-typed-field", hso["additionalContext"])
	assert.Equal(t, "preserved", hso["customKey"])
}

func TestBuildHookResponse_ReloadSkills_True(t *testing.T) {
	v := true
	resp := buildHookResponse("SessionStart", HookResult{
		Continue:     true,
		ReloadSkills: &v,
	})

	hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "SessionStart", hso["hookEventName"])
	assert.Equal(t, true, hso["reloadSkills"])
}

func TestBuildHookResponse_ReloadSkills_False(t *testing.T) {
	v := false
	resp := buildHookResponse("SessionStart", HookResult{
		Continue:     true,
		ReloadSkills: &v,
	})

	hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, hso["reloadSkills"])
}

func TestBuildHookResponse_ReloadSkills_Nil(t *testing.T) {
	resp := buildHookResponse("SessionStart", HookResult{
		Continue: true,
	})

	_, has := resp["hookSpecificOutput"]
	assert.False(t, has)
}

func TestBuildHookResponse_ReloadSkills_IgnoredOnPreToolUse(t *testing.T) {
	v := true
	resp := buildHookResponse("PreToolUse", HookResult{
		Continue:     true,
		ReloadSkills: &v,
	})

	_, has := resp["hookSpecificOutput"]
	assert.False(t, has)
}

func TestBuildHookResponse_ReloadSkills_ComposesWithAdditionalContext(t *testing.T) {
	v := true
	result := HookResult{
		Continue:     true,
		ReloadSkills: &v,
		HookSpecificOutput: map[string]interface{}{
			"hookEventName":     "SessionStart",
			"additionalContext": "skills installed",
		},
	}

	resp := buildHookResponse("SessionStart", result)

	hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "SessionStart", hso["hookEventName"])
	assert.Equal(t, "skills installed", hso["additionalContext"])
	assert.Equal(t, true, hso["reloadSkills"])

	_, mutated := result.HookSpecificOutput["reloadSkills"]
	assert.False(t, mutated)
}

func TestGetHookInput_MessageDisplay(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		ev, ok := input.(MessageDisplayInput)
		require.True(t, ok)
		assert.Equal(t, HookTypeMessageDisplay, ev.HookType())
		assert.Equal(t, "turn-123", ev.TurnID)
		assert.Equal(t, "message-456", ev.MessageID)
		assert.Equal(t, 2, ev.Index)
		assert.True(t, ev.Final)
		assert.Equal(t, "newly completed line\n", ev.Delta)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
		RequestID: "r",
		Request: SDKControlRequestBody{
			Subtype:    "hook_callback",
			CallbackID: "h",
			Input: map[string]interface{}{
				"hook_event_name": "MessageDisplay",
				"turn_id":         "turn-123",
				"message_id":      "message-456",
				"index":           float64(2),
				"final":           true,
				"delta":           "newly completed line\n",
			},
		},
	})
	assert.Equal(t, "success", resp.Response.Subtype)
}

func TestBuildHookResponse_MessageDisplay_DisplayContentSet(t *testing.T) {
	replacement := "replacement"
	resp := buildHookResponse("MessageDisplay", HookResult{
		Continue:       true,
		DisplayContent: &replacement,
	})

	hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "MessageDisplay", hso["hookEventName"])
	assert.Equal(t, "replacement", hso["displayContent"])
}

func TestBuildHookResponse_MessageDisplay_DisplayContentNil(t *testing.T) {
	resp := buildHookResponse("MessageDisplay", HookResult{
		Continue: true,
	})

	_, has := resp["hookSpecificOutput"]
	assert.False(t, has)
}

func TestBuildHookResponse_DisplayContentIgnoredOnNonMessageDisplay(t *testing.T) {
	replacement := "x"
	resp := buildHookResponse("PostToolUse", HookResult{
		Continue:       true,
		DisplayContent: &replacement,
	})

	_, has := resp["hookSpecificOutput"]
	assert.False(t, has)
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

	t.Run("WorktreeCreate is not watchPaths-eligible", func(t *testing.T) {
		// WorktreeCreateHookSpecificOutput (sdk.d.ts L5423-L5426) only
		// declares hookEventName and worktreePath. WatchPaths set on
		// this event must NOT leak into the wire response.
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
		_, hasWatchPaths := hso["watchPaths"]
		assert.False(t, hasWatchPaths, "WorktreeCreate output must not carry watchPaths")

		// Caller's map must not have been mutated by buildHookResponse.
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

func TestGetHookInput_SessionStart_SessionTitle(t *testing.T) {
	title := "My Session"

	assertLegacyHookInput(t, map[string]interface{}{
		"hook_event":    "SessionStart",
		"source":        "startup",
		"session_title": title,
	}, func(input HookInput) {
		ev, ok := input.(SessionStartInput)
		require.True(t, ok)
		require.NotNil(t, ev.SessionTitle)
		assert.Equal(t, title, *ev.SessionTitle)
	})

	assertSDKHookInput(t, map[string]interface{}{
		"hook_event_name": "SessionStart",
		"source":          "startup",
		"session_title":   title,
	}, func(input HookInput) {
		ev, ok := input.(SessionStartInput)
		require.True(t, ok)
		require.NotNil(t, ev.SessionTitle)
		assert.Equal(t, title, *ev.SessionTitle)
	})
}

func TestGetHookInput_SessionStart_NoSessionTitle(t *testing.T) {
	assertLegacyHookInput(t, map[string]interface{}{
		"hook_event": "SessionStart",
		"source":     "startup",
	}, func(input HookInput) {
		ev, ok := input.(SessionStartInput)
		require.True(t, ok)
		assert.Nil(t, ev.SessionTitle)
	})

	assertSDKHookInput(t, map[string]interface{}{
		"hook_event_name": "SessionStart",
		"source":          "startup",
	}, func(input HookInput) {
		ev, ok := input.(SessionStartInput)
		require.True(t, ok)
		assert.Nil(t, ev.SessionTitle)
	})
}

func TestGetHookInput_UserPromptSubmit_SessionTitle(t *testing.T) {
	title := "My Session"

	assertLegacyHookInput(t, map[string]interface{}{
		"hook_event":    "UserPromptSubmit",
		"prompt":        "hello",
		"session_title": title,
	}, func(input HookInput) {
		ev, ok := input.(UserPromptSubmitInput)
		require.True(t, ok)
		require.NotNil(t, ev.SessionTitle)
		assert.Equal(t, title, *ev.SessionTitle)
	})

	assertSDKHookInput(t, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello",
		"session_title":   title,
	}, func(input HookInput) {
		ev, ok := input.(UserPromptSubmitInput)
		require.True(t, ok)
		require.NotNil(t, ev.SessionTitle)
		assert.Equal(t, title, *ev.SessionTitle)
	})
}

func TestGetHookInput_UserPromptSubmit_NoSessionTitle(t *testing.T) {
	assertLegacyHookInput(t, map[string]interface{}{
		"hook_event": "UserPromptSubmit",
		"prompt":     "hello",
	}, func(input HookInput) {
		ev, ok := input.(UserPromptSubmitInput)
		require.True(t, ok)
		assert.Nil(t, ev.SessionTitle)
	})

	assertSDKHookInput(t, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello",
	}, func(input HookInput) {
		ev, ok := input.(UserPromptSubmitInput)
		require.True(t, ok)
		assert.Nil(t, ev.SessionTitle)
	})
}

func TestGetHookInput_UserPromptSubmit_Source(t *testing.T) {
	assertSDKHookInput(t, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello",
		"source":          "schedule_wakeup",
	}, func(input HookInput) {
		ev, ok := input.(UserPromptSubmitInput)
		require.True(t, ok)
		assert.Equal(t, "schedule_wakeup", ev.Source)
	})

	// Absent source leaves the field empty.
	assertSDKHookInput(t, map[string]interface{}{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "hello",
	}, func(input HookInput) {
		ev, ok := input.(UserPromptSubmitInput)
		require.True(t, ok)
		assert.Empty(t, ev.Source)
	})
}

func TestGetHookInput_SessionStart_ForkSource(t *testing.T) {
	assertSDKHookInput(t, map[string]interface{}{
		"hook_event_name": "SessionStart",
		"source":          "fork",
	}, func(input HookInput) {
		ev, ok := input.(SessionStartInput)
		require.True(t, ok)
		assert.Equal(t, "fork", ev.Source)
	})
}

func assertLegacyHookInput(t *testing.T, inputData map[string]interface{}, assertInput func(HookInput)) {
	t.Helper()

	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
	protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		assertInput(input)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleHookCallback(context.Background(), ControlRequest{
		RequestID: "r",
		Payload: map[string]interface{}{
			"callback_id": "h",
			"input":       inputData,
		},
	})
	assert.Equal(t, "success", resp.Response.Subtype)
}

func assertSDKHookInput(t *testing.T, inputData map[string]interface{}, assertInput func(HookInput)) {
	t.Helper()

	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
	protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		assertInput(input)
		return HookResult{Continue: true}, nil
	}

	resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
		RequestID: "r",
		Request: SDKControlRequestBody{
			Subtype:    "hook_callback",
			CallbackID: "h",
			Input:      inputData,
		},
	})
	assert.Equal(t, "success", resp.Response.Subtype)
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

	t.Run("DirectoryAdded via legacy path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(DirectoryAddedInput)
			require.True(t, ok)
			assert.Equal(t, "/repo/sub", ev.Directory)
			assert.Equal(t, "slash_command", ev.Source)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleHookCallback(context.Background(), ControlRequest{
			RequestID: "r",
			Payload: map[string]interface{}{
				"callback_id": "h",
				"input": map[string]interface{}{
					"hook_event": "DirectoryAdded",
					"directory":  "/repo/sub",
					"source":     "slash_command",
				},
			},
		})
		assert.Equal(t, "success", resp.Response.Subtype)
	})

	t.Run("DirectoryAdded via SDK path", func(t *testing.T) {
		runner := NewMockSubprocessRunner()
		opts := NewOptions()
		protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)
		protocol.hookCallbacks["h"] = func(ctx context.Context, input HookInput) (HookResult, error) {
			ev, ok := input.(DirectoryAddedInput)
			require.True(t, ok)
			assert.Equal(t, "/repo/sub", ev.Directory)
			assert.Equal(t, "register_repo_root", ev.Source)
			return HookResult{Continue: true}, nil
		}
		resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
			RequestID: "r",
			Request: SDKControlRequestBody{
				Subtype:    "hook_callback",
				CallbackID: "h",
				Input: map[string]interface{}{
					"hook_event_name": "DirectoryAdded",
					"directory":       "/repo/sub",
					"source":          "register_repo_root",
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

func TestBuildHookResponse_ClassifierContext(t *testing.T) {
	t.Run("emitted on PostToolUse", func(t *testing.T) {
		resp := buildHookResponse("PostToolUse", HookResult{
			Continue:          true,
			ClassifierContext: "the user asked for this deletion",
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "PostToolUse", hso["hookEventName"])
		assert.Equal(t, "the user asked for this deletion",
			hso["classifierContext"])
	})

	t.Run("empty omits the field", func(t *testing.T) {
		resp := buildHookResponse("PostToolUse", HookResult{
			Continue:          true,
			ClassifierContext: "",
		})

		_, has := resp["hookSpecificOutput"]
		assert.False(t, has)
	})

	t.Run("dropped on non-PostToolUse hooks", func(t *testing.T) {
		// The classifier reads this with host-application framing, so
		// leaking it onto an envelope that never declared it is worse
		// than dropping it.
		for _, hookType := range []string{
			"PreToolUse", "PostToolUseFailure", "UserPromptSubmit", "Stop",
		} {
			resp := buildHookResponse(hookType, HookResult{
				Continue:          true,
				ClassifierContext: "context",
			})

			_, has := resp["hookSpecificOutput"]
			assert.False(t, has,
				"classifierContext must not leak onto %s", hookType)
		}
	})

	t.Run("rides alongside a rewrite in the same result", func(t *testing.T) {
		// The upstream contract: pairing an assertion with the rewrite it
		// describes requires both in one hook result, so the assertion is
		// dropped if the rewrite is rejected or superseded.
		resp := buildHookResponse("PostToolUse", HookResult{
			Continue:          true,
			UpdatedToolOutput: "redacted output",
			ClassifierContext: "secrets stripped at the source",
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "PostToolUse", hso["hookEventName"])
		assert.Equal(t, "redacted output", hso["updatedToolOutput"])
		assert.Equal(t, "secrets stripped at the source",
			hso["classifierContext"])
	})

	t.Run("composes with explicit HookSpecificOutput", func(t *testing.T) {
		resp := buildHookResponse("PostToolUse", HookResult{
			Continue:          true,
			ClassifierContext: "typed wins",
			HookSpecificOutput: map[string]interface{}{
				"hookEventName":     "PostToolUse",
				"classifierContext": "raw loses",
				"additionalContext": "preserved",
			},
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "typed wins", hso["classifierContext"])
		assert.Equal(t, "preserved", hso["additionalContext"])
	})
}

func TestBuildHookResponse_SuppressOriginalPrompt_UserPromptExpansion(t *testing.T) {
	t.Run("emitted with the expansion event name", func(t *testing.T) {
		v := true
		resp := buildHookResponse("UserPromptExpansion", HookResult{
			Continue:               true,
			SuppressOriginalPrompt: &v,
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		// The envelope must name the hook that produced it, not the
		// UserPromptSubmit literal the gate used to hardcode.
		assert.Equal(t, "UserPromptExpansion", hso["hookEventName"])
		assert.Equal(t, true, hso["suppressOriginalPrompt"])
	})

	t.Run("explicit false still reaches the wire", func(t *testing.T) {
		v := false
		resp := buildHookResponse("UserPromptExpansion", HookResult{
			Continue:               true,
			SuppressOriginalPrompt: &v,
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, false, hso["suppressOriginalPrompt"])
	})

	t.Run("UserPromptSubmit still names itself", func(t *testing.T) {
		v := true
		resp := buildHookResponse("UserPromptSubmit", HookResult{
			Continue:               true,
			SuppressOriginalPrompt: &v,
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "UserPromptSubmit", hso["hookEventName"])
		assert.Equal(t, true, hso["suppressOriginalPrompt"])
	})
}

// TestHandleHookCallback_PreModelSwitchInput exercises the legacy hook-callback
// builder for the PreModelSwitch event added in TS SDK v0.3.251.
func TestHandleHookCallback_PreModelSwitchInput(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["hook_pre_switch"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		in, ok := input.(PreModelSwitchInput)
		require.True(t, ok)
		assert.Equal(t, "claude-sonnet-4-6", in.FromModel)
		assert.Equal(t, "claude-opus-4-8", in.ToModel)
		require.NotNil(t, in.RequestedModel)
		assert.Equal(t, "opus", *in.RequestedModel)
		assert.Equal(t, ModelSwitchSourceCommand, in.Source)
		assert.Equal(t, 148000, in.ContextTokens)
		assert.True(t, in.PromptCacheWarm)
		assert.Equal(t, CacheTTL1h, in.CacheTTL)
		assert.InDelta(t, 2.22, in.EstimatedCacheWriteUSD, 1e-9)
		assert.Equal(t, ModelPricingCatalog, in.Pricing)
		return HookResult{
			Continue:                 true,
			PermissionDecision:       HookPermissionDeny,
			PermissionDecisionReason: "cache write over budget",
		}, nil
	}

	resp := protocol.handleHookCallback(context.Background(), ControlRequest{
		Type:      "control",
		Subtype:   "hook_callback",
		RequestID: "req_pre_switch",
		Payload: map[string]interface{}{
			"callback_id": "hook_pre_switch",
			"input": map[string]interface{}{
				"hook_event":                "PreModelSwitch",
				"from_model":                "claude-sonnet-4-6",
				"to_model":                  "claude-opus-4-8",
				"requested_model":           "opus",
				"source":                    "command",
				"context_tokens":            float64(148000),
				"prompt_cache_warm":         true,
				"cache_ttl":                 "1h",
				"estimated_cache_write_usd": 2.22,
				"pricing":                   "catalog",
			},
		},
	})

	require.Equal(t, "success", resp.Response.Subtype)
	hso, ok := resp.Response.Response["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok, "a PreModelSwitch veto must ride hookSpecificOutput")
	assert.Equal(t, "PreModelSwitch", hso["hookEventName"])
	assert.Equal(t, "deny", hso["permissionDecision"])
	assert.Equal(t, "cache write over budget", hso["permissionDecisionReason"])
}

// TestHandleSDKHookCallback_PostModelSwitchInput exercises the SDK
// hook-callback builder, which is a separate field-mapping switch from the
// legacy one and drifts independently.
func TestHandleSDKHookCallback_PostModelSwitchInput(t *testing.T) {
	runner := NewMockSubprocessRunner()
	opts := NewOptions()
	protocol := NewProtocol(NewSubprocessTransportWithRunner(runner, opts), opts)

	protocol.hookCallbacks["sdk_post_switch"] = func(ctx context.Context, input HookInput) (HookResult, error) {
		in, ok := input.(PostModelSwitchInput)
		require.True(t, ok)
		assert.Equal(t, "claude-opus-4-8", in.FromModel)
		assert.Equal(t, "claude-sonnet-4-6", in.ToModel)
		assert.Nil(t, in.RequestedModel, "a resume names no requested model")
		assert.Equal(t, ModelSwitchSourceResume, in.Source)
		assert.False(t, in.PromptCacheWarm)
		assert.Equal(t, CacheTTL5m, in.CacheTTL)
		assert.Equal(t, ModelPricingConfigured, in.Pricing)
		return HookResult{Continue: true, AdditionalContext: "restored after resume"}, nil
	}

	resp := protocol.handleSDKHookCallback(context.Background(), SDKControlRequest{
		Type:      "control_request",
		RequestID: "sdk_post_switch",
		Request: SDKControlRequestBody{
			Subtype:    "hook_callback",
			CallbackID: "sdk_post_switch",
			Input: map[string]interface{}{
				"hook_event_name":           "PostModelSwitch",
				"from_model":                "claude-opus-4-8",
				"to_model":                  "claude-sonnet-4-6",
				"source":                    "resume",
				"context_tokens":            float64(9000),
				"prompt_cache_warm":         false,
				"cache_ttl":                 "5m",
				"estimated_cache_write_usd": 0.14,
				"pricing":                   "configured",
			},
		},
	})

	require.Equal(t, "success", resp.Response.Subtype)
	hso, ok := resp.Response.Response["hookSpecificOutput"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "PostModelSwitch", hso["hookEventName"])
	assert.Equal(t, "restored after resume", hso["additionalContext"])
}

func TestBuildHookResponse_PermissionDecision(t *testing.T) {
	t.Run("emitted on PreToolUse", func(t *testing.T) {
		resp := buildHookResponse("PreToolUse", HookResult{
			Continue:           true,
			PermissionDecision: HookPermissionDefer,
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "defer", hso["permissionDecision"])
		_, hasReason := hso["permissionDecisionReason"]
		assert.False(t, hasReason, "an empty reason must not reach the wire")
	})

	t.Run("dropped on unsupported hook", func(t *testing.T) {
		resp := buildHookResponse("PostModelSwitch", HookResult{
			Continue:           true,
			PermissionDecision: HookPermissionDeny,
		})

		_, has := resp["hookSpecificOutput"]
		assert.False(t, has,
			"only pre-phase hooks gate an action; the post-phase cannot veto one")
	})

	t.Run("composes with an explicit hookSpecificOutput", func(t *testing.T) {
		resp := buildHookResponse("PreModelSwitch", HookResult{
			Continue:           true,
			PermissionDecision: HookPermissionAsk,
			HookSpecificOutput: map[string]interface{}{
				"hookEventName": "PreModelSwitch",
				"customKey":     "preserved",
			},
		})

		hso, ok := resp["hookSpecificOutput"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ask", hso["permissionDecision"])
		assert.Equal(t, "preserved", hso["customKey"])
	})
}
