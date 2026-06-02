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

// TestStreamReadFileWithEncoding asserts the encoding option threads into the
// wire request and that a base64 response is parsed back onto the struct.
func TestStreamReadFileWithEncoding(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{
			"contents": "aGVsbG8=",
			"absPath":  "/tmp/example.bin",
			"encoding": "base64",
		}),
	)

	var got *SDKControlReadFileResponse
	err := callWithTimeout(t, func(ctx context.Context) error {
		var err error
		got, err = stream.ReadFile(ctx, "example.bin", &ReadFileOptions{
			Encoding: "base64",
		})
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "aGVsbG8=", got.Contents)
	assert.Equal(t, "/tmp/example.bin", got.AbsPath)
	assert.Equal(t, "base64", got.Encoding)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"read_file","path":"example.bin","encoding":"base64"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

// TestStreamReadFileEncodingEmptyOmitted asserts an empty Encoding string
// elides the key on the wire.
func TestStreamReadFileEncodingEmptyOmitted(t *testing.T) {
	stream, transport, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{}),
	)

	err := callWithTimeout(t, func(ctx context.Context) error {
		_, err := stream.ReadFile(ctx, "example.txt", &ReadFileOptions{
			Encoding: "",
		})
		return err
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"read_file","path":"example.txt"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

// TestStreamReadFileResponseEncodingDefaultsUTF8 asserts a response missing the
// encoding field unmarshals to an empty Encoding string.
func TestStreamReadFileResponseEncodingDefaultsUTF8(t *testing.T) {
	stream, _, _ := newStreamControlTest(
		successSDKControlResponseWithPayload(map[string]interface{}{
			"contents": "hello",
			"absPath":  "/tmp/example.txt",
		}),
	)

	var got *SDKControlReadFileResponse
	err := callWithTimeout(t, func(ctx context.Context) error {
		var err error
		got, err = stream.ReadFile(ctx, "example.txt", &ReadFileOptions{
			Encoding: "utf-8",
		})
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "hello", got.Contents)
	assert.Empty(t, got.Encoding)
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

// TestStreamApplyFlagSettingsNullValue asserts that a nil interface{} value for
// a top-level key marshals to JSON null on the wire - the v0.3.150 contract
// for clearing a key from the flag layer.
func TestStreamApplyFlagSettingsNullValue(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ApplyFlagSettings(ctx, map[string]interface{}{
			"clearMe": nil,
		})
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"apply_flag_settings","settings":{"clearMe":null}}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

// TestStreamApplyFlagSettingsMixedNullValues asserts a mixed map with both
// concrete values and explicit nils round-trips with each key preserved and
// nil values rendered as JSON null.
func TestStreamApplyFlagSettingsMixedNullValues(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.ApplyFlagSettings(ctx, map[string]interface{}{
			"keep":  "value",
			"clear": nil,
			"num":   7,
		})
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"apply_flag_settings","settings":{"keep":"value","clear":null,"num":7}}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamSubmitFeedbackMinimal(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.SubmitFeedback(ctx, "ship it")
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"submit_feedback","description":"ship it"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamSubmitFeedbackWithSurface(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.SubmitFeedback(ctx, "looks good",
			SubmitFeedbackOptions{Surface: "rating-thumb"})
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"submit_feedback","description":"looks good","surface":"rating-thumb"}}`,
		rawWrittenSDKControlRequest(t, transport),
	)
}

func TestStreamSubmitFeedbackOmitsEmptySurface(t *testing.T) {
	stream, transport, _ := newStreamControlTest(successSDKControlResponse)

	err := callWithTimeout(t, func(ctx context.Context) error {
		return stream.SubmitFeedback(ctx, "hi", SubmitFeedbackOptions{})
	})
	require.NoError(t, err)

	assert.JSONEq(t,
		`{"type":"control_request","request_id":"req_1","request":{"subtype":"submit_feedback","description":"hi"}}`,
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

func TestStreamBackgroundTasksWireShape(t *testing.T) {
	t.Run("with tool_use_id", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(
			successSDKControlResponseWithPayload(map[string]interface{}{
				"backgrounded": true,
			}),
		)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ok, err := stream.BackgroundTasks(ctx, "tool_use_42")
		require.NoError(t, err)
		assert.True(t, ok)

		assert.JSONEq(t,
			`{"type":"control_request","request_id":"req_1",`+
				`"request":{"subtype":"background_tasks",`+
				`"tool_use_id":"tool_use_42"}}`,
			rawWrittenSDKControlRequest(t, transport),
		)
	})

	t.Run("without tool_use_id", func(t *testing.T) {
		stream, transport, _ := newStreamControlTest(
			successSDKControlResponseWithPayload(map[string]interface{}{
				"backgrounded": true,
			}),
		)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ok, err := stream.BackgroundTasks(ctx, "")
		require.NoError(t, err)
		assert.True(t, ok)

		assert.JSONEq(t,
			`{"type":"control_request","request_id":"req_1",`+
				`"request":{"subtype":"background_tasks"}}`,
			rawWrittenSDKControlRequest(t, transport),
		)
	})

	t.Run("no match returns false", func(t *testing.T) {
		stream, _, _ := newStreamControlTest(
			successSDKControlResponseWithPayload(map[string]interface{}{
				"backgrounded": false,
			}),
		)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ok, err := stream.BackgroundTasks(ctx, "tool_use_does_not_exist")
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("absent backgrounded key defaults to true", func(t *testing.T) {
		stream, _, _ := newStreamControlTest(successSDKControlResponse)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		ok, err := stream.BackgroundTasks(ctx, "")
		require.NoError(t, err)
		assert.True(t, ok)
	})
}
