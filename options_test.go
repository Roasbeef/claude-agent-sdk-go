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
