package claudeagent

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type streamControlTransport struct {
	mu       sync.Mutex
	written  []Message
	writeCh  chan Message
	protocol *Protocol
	response func(SDKControlRequest) SDKControlResponse
	closed   atomic.Bool
	ready    atomic.Bool
}

func newStreamControlTransport(
	response func(SDKControlRequest) SDKControlResponse,
) *streamControlTransport {
	return &streamControlTransport{
		writeCh:  make(chan Message, 8),
		response: response,
	}
}

func (t *streamControlTransport) Connect(ctx context.Context) error {
	t.ready.Store(true)
	return nil
}

func (t *streamControlTransport) Write(ctx context.Context, msg Message) error {
	if t.closed.Load() {
		return &ErrTransportClosed{}
	}

	t.mu.Lock()
	t.written = append(t.written, msg)
	t.mu.Unlock()

	select {
	case t.writeCh <- msg:
	default:
	}

	if t.response != nil {
		var req SDKControlRequest
		data, err := json.Marshal(msg)
		if err == nil {
			err = json.Unmarshal(data, &req)
		}
		if err == nil {
			resp := t.response(req)
			go func() {
				_ = t.protocol.handleSDKControlResponse(resp)
			}()
		}
	}

	return nil
}

func (t *streamControlTransport) ReadMessages(ctx context.Context) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		<-ctx.Done()
	}
}

func (t *streamControlTransport) EndInput() error { return nil }

func (t *streamControlTransport) Close() error {
	t.closed.Store(true)
	return nil
}

func (t *streamControlTransport) IsReady() bool {
	return t.ready.Load() && !t.closed.Load()
}

func (t *streamControlTransport) writtenMessages() []Message {
	t.mu.Lock()
	defer t.mu.Unlock()

	out := make([]Message, len(t.written))
	copy(out, t.written)
	return out
}

func newStreamControlTest(
	response func(SDKControlRequest) SDKControlResponse,
) (*Stream, *streamControlTransport, *Protocol) {
	transport := newStreamControlTransport(response)
	options := DefaultOptions()
	protocol := NewProtocol(transport, &options)
	transport.protocol = protocol
	client := &Client{
		options:   options,
		transport: transport,
		protocol:  protocol,
	}
	stream := &Stream{
		client:  client,
		ctx:     context.Background(),
		sendCh:  make(chan UserMessage),
		closeCh: make(chan struct{}),
	}
	return stream, transport, protocol
}

func successSDKControlResponse(req SDKControlRequest) SDKControlResponse {
	return SDKControlResponse{
		Type: "control_response",
		Response: SDKControlResponseBody{
			Subtype:   "success",
			RequestID: req.RequestID,
			Response:  map[string]interface{}{},
		},
	}
}

func decodeWrittenSDKControlRequest(
	t *testing.T, transport *streamControlTransport,
) (SDKControlRequest, map[string]interface{}) {
	t.Helper()

	written := transport.writtenMessages()
	require.Len(t, written, 1)

	data, err := json.Marshal(written[0])
	require.NoError(t, err)

	var req SDKControlRequest
	require.NoError(t, json.Unmarshal(data, &req))

	var generic map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &generic))
	return req, generic
}

func genericRequestBody(t *testing.T, generic map[string]interface{}) map[string]interface{} {
	t.Helper()

	body, ok := generic["request"].(map[string]interface{})
	require.True(t, ok, "request body missing from %+v", generic)
	return body
}

func callWithTimeout(t *testing.T, fn func(context.Context) error) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return fn(ctx)
}

func withResult[T any](t *testing.T, fn func(context.Context) (T, error)) (T, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return fn(ctx)
}

func TestStreamInterruptSendsControlRequest(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	require.NoError(t, callWithTimeout(t, stream.Interrupt))

	req, generic := decodeWrittenSDKControlRequest(t, transport)
	assert.Equal(t, "control_request", req.Type)
	assert.NotEmpty(t, req.RequestID)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "interrupt", body["subtype"])
	assert.NotContains(t, body, "mode")
	assert.NotContains(t, body, "model")
	assert.NotContains(t, body, "max_thinking_tokens")
	// A plain interrupt must not set cancel_queued.
	assert.NotContains(t, body, "cancel_queued")
}

func TestStreamInterruptWithReceiptStillQueued(t *testing.T) {
	stream, _, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{
			"still_queued": []string{"uuid-a", "uuid-b"},
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	receipt, err := stream.InterruptWithReceipt(ctx)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Equal(t, []string{"uuid-a", "uuid-b"}, receipt.StillQueued)
}

func TestStreamInterruptWithReceiptOlderCLI(t *testing.T) {
	// Older CLIs send an empty success response with no still_queued field.
	stream, _, _ := newStreamControlTest(successSDKControlResponse)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	receipt, err := stream.InterruptWithReceipt(ctx)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Empty(t, receipt.StillQueued)
}

func TestStreamInterruptCancelQueued(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{
			"still_queued": []string{},
			"cancelled":    []string{"uuid-a", "uuid-b"},
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	receipt, err := stream.InterruptCancelQueued(ctx)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	assert.Empty(t, receipt.StillQueued)
	assert.Equal(t, []string{"uuid-a", "uuid-b"}, receipt.Cancelled)

	// cancel_queued:true must be on the wire.
	_, generic := decodeWrittenSDKControlRequest(t, transport)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "interrupt", body["subtype"])
	assert.Equal(t, true, body["cancel_queued"])
}

func TestStreamSetPermissionModeSendsModeField(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.SetPermissionMode(ctx, PermissionModeAcceptEdits)
	})
	require.NoError(t, err)

	_, generic := decodeWrittenSDKControlRequest(t, transport)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "set_permission_mode", body["subtype"])
	assert.Equal(t, "acceptEdits", body["mode"])
}

func TestStreamSetModelSendsModelField(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.SetModel(ctx, "claude-sonnet-4-5-20250929")
	})
	require.NoError(t, err)

	_, generic := decodeWrittenSDKControlRequest(t, transport)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "set_model", body["subtype"])
	assert.Equal(t, "claude-sonnet-4-5-20250929", body["model"])
}

func TestStreamSetMcpPermissionModeOverrideRoundTrip(t *testing.T) {
	defaultMode := McpPermissionOverrideModeDefault
	autoMode := McpPermissionOverrideModeAuto

	tests := []struct {
		name     string
		mode     *McpPermissionOverrideMode
		wantMode interface{} // string for a value, nil for explicit JSON null
	}{
		{name: "default", mode: &defaultMode, wantMode: "default"},
		{name: "auto", mode: &autoMode, wantMode: "auto"},
		{name: "clear sends null", mode: nil, wantMode: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, transport, _ := newStreamControlTest(successSDKControlResponse)

			_, err := withResult(t, func(ctx context.Context) (McpPermissionModeOverrideResult, error) {
				return stream.SetMcpPermissionModeOverride(ctx, "my-server", tt.mode)
			})
			require.NoError(t, err)

			_, generic := decodeWrittenSDKControlRequest(t, transport)
			body := genericRequestBody(t, generic)
			assert.Equal(t, "set_mcp_permission_mode_override", body["subtype"])
			assert.Equal(t, "my-server", body["serverName"])
			// mode key is always present (tristate): a string, or explicit null.
			require.Contains(t, body, "mode")
			assert.Equal(t, tt.wantMode, body["mode"])
		})
	}
}

func TestStreamSetMcpPermissionModeOverrideParsesWarning(t *testing.T) {
	warnResponse := func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response:  map[string]interface{}{"warning": "unknown server"},
			},
		}
	}
	stream, _, _ := newStreamControlTest(warnResponse)

	mode := McpPermissionOverrideModeDefault
	result, err := withResult(t, func(ctx context.Context) (McpPermissionModeOverrideResult, error) {
		return stream.SetMcpPermissionModeOverride(ctx, "typo-server", &mode)
	})
	require.NoError(t, err)
	assert.Equal(t, "unknown server", result.Warning)
}

func TestStreamSetMaxThinkingTokensRoundTrip(t *testing.T) {
	// The key is always sent. A nil budget must go out as an explicit null:
	// since v0.3.290 the CLI reads an absent key as "leave the budget as it
	// is", so omitting it would silently turn the reset into a no-op.
	tests := []struct {
		name   string
		tokens *int
		want   interface{}
	}{
		{name: "nil sends null", tokens: nil, want: nil},
		{name: "zero present", tokens: intPtr(0), want: float64(0)},
		{name: "nonzero present", tokens: intPtr(4096), want: float64(4096)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, transport, _ := newStreamControlTest(successSDKControlResponse)

			err := callWithTimeout(t, func(ctx context.Context) error {
				return stream.SetMaxThinkingTokens(ctx, tt.tokens)
			})
			require.NoError(t, err)

			_, generic := decodeWrittenSDKControlRequest(t, transport)
			body := genericRequestBody(t, generic)
			assert.Equal(t, "set_max_thinking_tokens", body["subtype"])

			got, ok := body["max_thinking_tokens"]
			require.True(t, ok, "max_thinking_tokens should be present")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStreamSetMaxThinkingTokensThinkingDisplay(t *testing.T) {
	t.Run("omitted when no option", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(successSDKControlResponse)
		err := callWithTimeout(t, func(ctx context.Context) error {
			return stream.SetMaxThinkingTokens(ctx, intPtr(4096))
		})
		require.NoError(t, err)

		_, generic := decodeWrittenSDKControlRequest(t, transport)
		body := genericRequestBody(t, generic)
		assert.NotContains(t, body, "thinking_display")
	})

	t.Run("mode value", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(successSDKControlResponse)
		err := callWithTimeout(t, func(ctx context.Context) error {
			return stream.SetMaxThinkingTokens(ctx, nil, WithThinkingDisplay(ThinkingDisplayOmitted))
		})
		require.NoError(t, err)

		_, generic := decodeWrittenSDKControlRequest(t, transport)
		body := genericRequestBody(t, generic)
		assert.Equal(t, "omitted", body["thinking_display"])
	})

	t.Run("api default is explicit null", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(successSDKControlResponse)
		err := callWithTimeout(t, func(ctx context.Context) error {
			return stream.SetMaxThinkingTokens(ctx, nil, WithThinkingDisplayAPIDefault())
		})
		require.NoError(t, err)

		_, generic := decodeWrittenSDKControlRequest(t, transport)
		body := genericRequestBody(t, generic)
		got, ok := body["thinking_display"]
		require.True(t, ok, "thinking_display should be present as null")
		assert.Nil(t, got)
	})
}

func TestStreamRenameSession(t *testing.T) {
	t.Run("title only omits source and session_id", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(successSDKControlResponse)

		err := callWithTimeout(t, func(ctx context.Context) error {
			return stream.RenameSession(ctx, "New title")
		})
		require.NoError(t, err)

		_, generic := decodeWrittenSDKControlRequest(t, transport)
		body := genericRequestBody(t, generic)
		assert.Equal(t, "rename_session", body["subtype"])
		assert.Equal(t, "New title", body["title"])
		assert.NotContains(t, body, "source")
		assert.NotContains(t, body, "session_id")
	})

	t.Run("source and session guard carried through", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(successSDKControlResponse)

		err := callWithTimeout(t, func(ctx context.Context) error {
			return stream.RenameSession(ctx, "Guarded title",
				WithRenameSource(RenameSessionSourceHost),
				WithRenameSessionID("sess-123"))
		})
		require.NoError(t, err)

		_, generic := decodeWrittenSDKControlRequest(t, transport)
		body := genericRequestBody(t, generic)
		assert.Equal(t, "rename_session", body["subtype"])
		assert.Equal(t, "Guarded title", body["title"])
		assert.Equal(t, "host", body["source"])
		assert.Equal(t, "sess-123", body["session_id"])
	})
}

func TestStreamRegisterRepoRootMinimal(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.RegisterRepoRoot(ctx, "packages/app")
	})
	require.NoError(t, err)

	_, generic := decodeWrittenSDKControlRequest(t, transport)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "register_repo_root", body["subtype"])
	assert.Equal(t, "packages/app", body["directory"])
	assert.NotContains(t, body, "reload_claude_md")
	assert.NotContains(t, body, "reload_plugins")
	assert.NotContains(t, body, "reload_skills")
}

func TestStreamRegisterRepoRootAllFlags(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.RegisterRepoRoot(ctx, "packages/app",
			WithReloadClaudeMD(),
			WithReloadPlugins(),
			WithReloadSkills(),
		)
	})
	require.NoError(t, err)

	_, generic := decodeWrittenSDKControlRequest(t, transport)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "register_repo_root", body["subtype"])
	assert.Equal(t, "packages/app", body["directory"])
	assert.Equal(t, true, body["reload_claude_md"])
	assert.Equal(t, true, body["reload_plugins"])
	assert.Equal(t, true, body["reload_skills"])
}

func TestStreamRegisterRepoRootSomeFlagsFalse(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.RegisterRepoRoot(ctx, "packages/app",
			WithReloadClaudeMD(false),
			WithReloadPlugins(false),
		)
	})
	require.NoError(t, err)

	_, generic := decodeWrittenSDKControlRequest(t, transport)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "register_repo_root", body["subtype"])
	assert.Equal(t, "packages/app", body["directory"])
	assert.Equal(t, false, body["reload_claude_md"])
	assert.Equal(t, false, body["reload_plugins"])
	assert.NotContains(t, body, "reload_skills")
}

func TestStreamControlRequestSurfacesError(t *testing.T) {
	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "error",
				RequestID: req.RequestID,
				Error:     "invalid mode",
			},
		}
	})

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.SetPermissionMode(ctx, PermissionMode("invalid"))
	})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "invalid mode"))
}

func TestStreamControlRequestRespectsContextCancel(t *testing.T) {
	stream, transport, protocol := newStreamControlTest(nil)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- stream.Interrupt(ctx)
	}()

	var msg Message
	select {
	case msg = <-transport.writeCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for control request write")
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	var req SDKControlRequest
	require.NoError(t, json.Unmarshal(data, &req))

	_, exists := protocol.pendingReqs.Load(req.RequestID)
	require.True(t, exists, "pending request should be registered before cancellation")

	cancel()

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.Canceled))
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for control method to return")
	}

	_, exists = protocol.pendingReqs.Load(req.RequestID)
	assert.False(t, exists, "pending request should be cleaned up after cancellation")
}

// TestStreamSendMessage checks SendMessage writes the caller's fields through
// untouched and fills in only the envelope defaults it leaves empty.
func TestStreamSendMessage(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)
	stream.sessionID = "sess_send"
	go stream.handleSends()
	defer stream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, stream.SendMessage(ctx, UserMessage{
		UUID: "550e8400-e29b-41d4-a716-446655440900",
		Message: APIUserMessage{
			Content: []UserContentBlock{{Type: "text", Text: "explain: boom"}},
		},
		PastedContent:  []PastedContentEntry{{Text: "stack trace"}},
		InlinePastes:   []string{"boom"},
		ClientComposed: true,
	}))
	require.NoError(t, stream.Send(ctx, "plain"))

	var written []Message
	require.Eventually(t, func() bool {
		written = transport.writtenMessages()
		return len(written) == 2
	}, time.Second, 5*time.Millisecond)

	full := written[0].(UserMessage)
	assert.Equal(t, "user", full.Type)
	assert.Equal(t, "sess_send", full.SessionID)
	assert.Equal(t, "user", full.Message.Role)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440900", full.UUID)
	assert.Equal(t, []string{"boom"}, full.InlinePastes)
	assert.Equal(t, "stack trace", full.PastedContent[0].Text)
	assert.True(t, full.ClientComposed)

	plain := written[1].(UserMessage)
	assert.Equal(t, "plain", plain.Message.Content[0].Text)
	assert.False(t, plain.ClientComposed)
}

func TestStreamSendMessageAfterClose(t *testing.T) {
	stream, _, _ := newStreamControlTest(successSDKControlResponse)
	require.NoError(t, stream.Close())

	err := stream.SendMessage(context.Background(), UserMessage{})
	var closed *ErrTransportClosed
	assert.ErrorAs(t, err, &closed)
}
