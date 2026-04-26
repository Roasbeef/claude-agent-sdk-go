package claudeagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawWrittenSDKControlRequest(
	t *testing.T,
	transport *streamControlTransport,
) string {
	t.Helper()

	written := transport.writtenMessages()
	require.Len(t, written, 1)

	data, err := json.Marshal(written[0])
	require.NoError(t, err)
	return string(data)
}

func successSDKControlResponseWithPayload(
	payload map[string]interface{},
) func(SDKControlRequest) SDKControlResponse {
	return func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response:  payload,
			},
		}
	}
}

func TestStreamReconnectMcpServerWireShape(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ReconnectMcpServer(ctx, "foo")
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"mcp_reconnect","serverName":"foo"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamToggleMcpServerWireShapeTrue(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ToggleMcpServer(ctx, "foo", true)
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"mcp_toggle","serverName":"foo","enabled":true}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamToggleMcpServerWireShapeFalse(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ToggleMcpServer(ctx, "foo", false)
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"mcp_toggle","serverName":"foo","enabled":false}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamSetMcpServersWireShape(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{
			"added":   []interface{}{},
			"removed": []interface{}{},
			"errors":  map[string]interface{}{},
		}),
	)

	got, err := stream.SetMcpServers(context.Background(), map[string]MCPServerConfig{
		"myserver": {
			Type:    "stdio",
			Command: "node",
			Args:    []string{"server.js"},
			Env:     map[string]string{"TOKEN": "abc"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"mcp_set_servers","servers":{"myserver":{"type":"stdio","command":"node","args":["server.js"],"env":{"TOKEN":"abc"}}}}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamSetMcpServersEmpty(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]MCPServerConfig
	}{
		{name: "nil", servers: nil},
		{name: "empty", servers: map[string]MCPServerConfig{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, transport, _ := newStreamControlTest(
				successSDKControlResponseWithPayload(map[string]interface{}{
					"added":   []interface{}{},
					"removed": []interface{}{},
					"errors":  map[string]interface{}{},
				}),
			)

			got, err := stream.SetMcpServers(context.Background(), tt.servers)
			require.NoError(t, err)
			require.NotNil(t, got)

			assert.JSONEq(t,
				`{"type":"control_request","request_id":"req_1","request":{"subtype":"mcp_set_servers","servers":{}}}`,
				rawWrittenSDKControlRequest(t, transport),
			)
		})
	}
}

func TestStreamSetMcpServersParsesResult(t *testing.T) {
	stream, _, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{
			"added":   []interface{}{"a", "b"},
			"removed": []interface{}{"c"},
			"errors":  map[string]interface{}{"d": "timeout"},
		}),
	)

	got, err := stream.SetMcpServers(context.Background(), map[string]MCPServerConfig{})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, []string{"a", "b"}, got.Added)
	assert.Equal(t, []string{"c"}, got.Removed)
	assert.Equal(t, map[string]string{"d": "timeout"}, got.Errors)
}

func TestStreamSetMcpServersEmptyArrays(t *testing.T) {
	stream, _, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{
			"added":   []interface{}{},
			"removed": []interface{}{},
			"errors":  map[string]interface{}{},
		}),
	)

	got, err := stream.SetMcpServers(context.Background(), map[string]MCPServerConfig{})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotNil(t, got.Added)
	assert.Empty(t, got.Added)
	assert.NotNil(t, got.Removed)
	assert.Empty(t, got.Removed)
	assert.NotNil(t, got.Errors)
	assert.Empty(t, got.Errors)
}

func TestStreamReconnectMcpServerControlError(t *testing.T) {
	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "error",
				RequestID: req.RequestID,
				Error:     "unknown server",
			},
		}
	})

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ReconnectMcpServer(ctx, "foo")
	})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown server"))
}

func TestStreamToggleMcpServerControlError(t *testing.T) {
	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "error",
				RequestID: req.RequestID,
				Error:     "unknown server",
			},
		}
	})

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ToggleMcpServer(ctx, "foo", false)
	})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "unknown server"))
}
