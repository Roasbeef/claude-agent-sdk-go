package claudeagent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsHookArgsJSON(t *testing.T) {
	t.Run("marshal includes args", func(t *testing.T) {
		data, err := json.Marshal(SettingsHook{
			Type:    "command",
			Command: "foo",
			Args:    []string{"--flag", "value"},
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, []interface{}{"--flag", "value"}, got["args"])
	})

	t.Run("nil args omitted", func(t *testing.T) {
		data, err := json.Marshal(SettingsHook{
			Type:    "command",
			Command: "foo",
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "args")
	})

	t.Run("unmarshal args", func(t *testing.T) {
		var cfg SettingsHook
		require.NoError(t, json.Unmarshal(
			[]byte(`{"type":"command","command":"foo","args":["--flag","value"]}`),
			&cfg,
		))

		assert.Equal(t, []string{"--flag", "value"}, cfg.Args)
	})

	t.Run("empty args included", func(t *testing.T) {
		data, err := json.Marshal(SettingsHook{
			Type:    "command",
			Command: "foo",
			// Non-nil empty args intentionally forces exec form with no args.
			Args: []string{},
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, []interface{}{}, got["args"])
	})
}

func TestSettingsHookContinueOnBlockJSON(t *testing.T) {
	t.Run("marshal includes true", func(t *testing.T) {
		continueOnBlock := true
		data, err := json.Marshal(SettingsHook{
			Type:            "prompt",
			Prompt:          "verify",
			ContinueOnBlock: &continueOnBlock,
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, true, got["continueOnBlock"])
	})

	t.Run("nil omitted", func(t *testing.T) {
		data, err := json.Marshal(SettingsHook{
			Type:   "prompt",
			Prompt: "verify",
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "continueOnBlock")
	})

	t.Run("marshal includes false", func(t *testing.T) {
		continueOnBlock := false
		data, err := json.Marshal(SettingsHook{
			Type:            "prompt",
			Prompt:          "verify",
			ContinueOnBlock: &continueOnBlock,
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, false, got["continueOnBlock"])
	})

	t.Run("unmarshal", func(t *testing.T) {
		var cfg SettingsHook
		require.NoError(t, json.Unmarshal(
			[]byte(`{"type":"prompt","prompt":"verify","continueOnBlock":true}`),
			&cfg,
		))

		require.NotNil(t, cfg.ContinueOnBlock)
		assert.True(t, *cfg.ContinueOnBlock)
	})
}

func TestBaseHookInputEffortJSON(t *testing.T) {
	t.Run("marshal includes effort", func(t *testing.T) {
		data, err := json.Marshal(PreToolUseInput{
			BaseHookInput: BaseHookInput{
				SessionID:      "s",
				TranscriptPath: "/t",
				Cwd:            "/c",
				Effort:         &HookEffort{Level: EffortHigh},
			},
			ToolName: "Read",
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, map[string]interface{}{"level": "high"}, got["effort"])
	})

	t.Run("nil omitted", func(t *testing.T) {
		data, err := json.Marshal(PreToolUseInput{
			BaseHookInput: BaseHookInput{
				SessionID:      "s",
				TranscriptPath: "/t",
				Cwd:            "/c",
			},
			ToolName: "Read",
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "effort")
	})

	t.Run("marshal includes xhigh", func(t *testing.T) {
		data, err := json.Marshal(PreToolUseInput{
			BaseHookInput: BaseHookInput{
				SessionID:      "s",
				TranscriptPath: "/t",
				Cwd:            "/c",
				Effort:         &HookEffort{Level: EffortXHigh},
			},
			ToolName: "Read",
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, map[string]interface{}{"level": "xhigh"}, got["effort"])
	})

	t.Run("unmarshal round-trip", func(t *testing.T) {
		var input PreToolUseInput
		require.NoError(t, json.Unmarshal(
			[]byte(`{"session_id":"s","transcript_path":"/t","cwd":"/c","tool_name":"Read","effort":{"level":"high"}}`),
			&input,
		))

		require.NotNil(t, input.Effort)
		assert.Equal(t, EffortHigh, input.Effort.Level)
		assert.Equal(t, EffortHigh, input.Base().Effort.Level)
	})
}
