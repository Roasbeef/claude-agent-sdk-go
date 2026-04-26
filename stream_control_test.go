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
		sendCh:  make(chan string),
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

func TestStreamSetMaxThinkingTokensRoundTrip(t *testing.T) {
	tests := []struct {
		name        string
		tokens      *int
		wantPresent bool
		want        float64
	}{
		{name: "nil omitted", tokens: nil},
		{name: "zero present", tokens: intPtr(0), wantPresent: true, want: 0},
		{name: "nonzero present", tokens: intPtr(4096), wantPresent: true, want: 4096},
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
			if !tt.wantPresent {
				assert.False(t, ok, "max_thinking_tokens should be omitted")
				return
			}
			require.True(t, ok, "max_thinking_tokens should be present")
			assert.Equal(t, tt.want, got)
		})
	}
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
