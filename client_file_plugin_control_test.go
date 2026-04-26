package claudeagent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func controlErrorResponse(message string) func(SDKControlRequest) SDKControlResponse {
	return func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "error",
				RequestID: req.RequestID,
				Error:     message,
			},
		}
	}
}

func TestStreamRewindFilesNoOpts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(
			successSDKControlResponseWithPayload(map[string]interface{}{
				"canRewind":    true,
				"filesChanged": []interface{}{"a", "b"},
				"insertions":   float64(3),
			}),
		)

		var got *RewindFilesResult
		err := callWithTimeout(t, func(ctx context.Context) error {
			var err error
			got, err = stream.RewindFiles(ctx, "user-msg-1", nil)
			return err
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.True(t, got.CanRewind)
		assert.Equal(t, []string{"a", "b"}, got.FilesChanged)
		assert.Equal(t, 3, got.Insertions)

		assert.JSONEq(t,
			`{"type":"control_request","request_id":"req_1","request":{"subtype":"rewind_files","user_message_id":"user-msg-1"}}`,
			rawWrittenSDKControlRequest(t, transport),
		)
	})

	t.Run("error", func(t *testing.T) {
		stream, _, _ := newStreamControlTest(controlErrorResponse("checkpoint missing"))

		err := callWithTimeout(t, func(ctx context.Context) error {
			_, err := stream.RewindFiles(ctx, "user-msg-1", nil)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "checkpoint missing")
	})
}

func TestStreamRewindFilesDryRunTrue(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{}),
	)

	err := callWithTimeout(t, func(ctx context.Context) error {
		_, err := stream.RewindFiles(ctx, "user-msg-1", &RewindFilesOptions{
			DryRun: true,
		})
		return err
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"rewind_files","user_message_id":"user-msg-1","dry_run":true}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamRewindFilesDryRunFalseOmitted(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{}),
	)

	err := callWithTimeout(t, func(ctx context.Context) error {
		_, err := stream.RewindFiles(ctx, "user-msg-1", &RewindFilesOptions{
			DryRun: false,
		})
		return err
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"rewind_files","user_message_id":"user-msg-1"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamRewindFilesError(t *testing.T) {
	stream, _, _ := newStreamControlTest(controlErrorResponse("cannot rewind"))

	err := callWithTimeout(t, func(ctx context.Context) error {
		_, err := stream.RewindFiles(ctx, "user-msg-1", &RewindFilesOptions{
			DryRun: true,
		})
		return err
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot rewind")
}

func TestStreamSeedReadStateWireShape(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.SeedReadState(ctx, "a.go", 1700000000000)
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"seed_read_state","path":"a.go","mtime":1700000000000}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamReadFileNoOpts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(
			successSDKControlResponseWithPayload(map[string]interface{}{
				"contents":  "hello",
				"absPath":   "/tmp/example.txt",
				"truncated": true,
			}),
		)

		var got *SDKControlReadFileResponse
		err := callWithTimeout(t, func(ctx context.Context) error {
			var err error
			got, err = stream.ReadFile(ctx, "example.txt", nil)
			return err
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "hello", got.Contents)
		assert.Equal(t, "/tmp/example.txt", got.AbsPath)
		assert.True(t, got.Truncated)

		assert.JSONEq(t,
			`{"type":"control_request","request_id":"req_1","request":{"subtype":"read_file","path":"example.txt"}}`,
			rawWrittenSDKControlRequest(t, transport),
		)
	})

	t.Run("error", func(t *testing.T) {
		stream, _, _ := newStreamControlTest(controlErrorResponse("permission denied"))

		err := callWithTimeout(t, func(ctx context.Context) error {
			_, err := stream.ReadFile(ctx, "example.txt", nil)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})
}

func TestStreamReadFileWithMaxBytes(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{}),
	)

	err := callWithTimeout(t, func(ctx context.Context) error {
		_, err := stream.ReadFile(ctx, "example.txt", &ReadFileOptions{
			MaxBytes: 4096,
		})
		return err
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"read_file","path":"example.txt","max_bytes":4096}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamReadFileMaxBytesZeroOmitted(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{}),
	)

	err := callWithTimeout(t, func(ctx context.Context) error {
		_, err := stream.ReadFile(ctx, "example.txt", &ReadFileOptions{
			MaxBytes: 0,
		})
		return err
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"read_file","path":"example.txt"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamReloadPluginsParsesResponse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(
			successSDKControlResponseWithPayload(map[string]interface{}{
				"commands": []interface{}{
					map[string]interface{}{
						"name":         "review",
						"description":  "Run review",
						"argumentHint": "[target]",
					},
				},
				"agents": []interface{}{
					map[string]interface{}{
						"name":        "planner",
						"description": "Plans work",
						"model":       "sonnet",
					},
				},
				"plugins": []interface{}{
					map[string]interface{}{
						"name":   "local",
						"path":   "/tmp/plugin",
						"source": "project",
					},
				},
				"mcpServers": []interface{}{
					map[string]interface{}{
						"name":   "github",
						"status": "connected",
						"serverInfo": map[string]interface{}{
							"name":    "github",
							"version": "1.0.0",
						},
					},
				},
				"error_count": float64(2),
			}),
		)

		var got *SDKControlReloadPluginsResponse
		err := callWithTimeout(t, func(ctx context.Context) error {
			var err error
			got, err = stream.ReloadPlugins(ctx)
			return err
		})
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, []SlashCommand{{
			Name:         "review",
			Description:  "Run review",
			ArgumentHint: "[target]",
		}}, got.Commands)
		assert.Equal(t, []AgentInfo{{
			Name:        "planner",
			Description: "Plans work",
			Model:       "sonnet",
		}}, got.Agents)
		assert.Equal(t, []PluginInfo{{
			Name:   "local",
			Path:   "/tmp/plugin",
			Source: "project",
		}}, got.Plugins)
		require.Len(t, got.McpServers, 1)
		assert.Equal(t, "github", got.McpServers[0].Name)
		assert.Equal(t, McpServerStateConnected, got.McpServers[0].Status)
		require.NotNil(t, got.McpServers[0].ServerInfo)
		assert.Equal(t, "1.0.0", got.McpServers[0].ServerInfo.Version)
		assert.Equal(t, 2, got.ErrorCount)

		assert.JSONEq(t,
			`{"type":"control_request","request_id":"req_1","request":{"subtype":"reload_plugins"}}`,
			rawWrittenSDKControlRequest(t, transport),
		)
	})

	t.Run("error", func(t *testing.T) {
		stream, _, _ := newStreamControlTest(controlErrorResponse("reload failed"))

		err := callWithTimeout(t, func(ctx context.Context) error {
			_, err := stream.ReloadPlugins(ctx)
			return err
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reload failed")
	})
}

func TestStreamApplyFlagSettingsNonEmpty(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ApplyFlagSettings(ctx, map[string]interface{}{
			"foo": "bar",
			"n":   42,
		})
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"apply_flag_settings","settings":{"foo":"bar","n":42}}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamApplyFlagSettingsNil(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ApplyFlagSettings(ctx, nil)
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"apply_flag_settings","settings":{}}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamStopTaskWireShape(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, stream.StopTask(ctx, "task-123"))

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"stop_task","task_id":"task-123"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}
