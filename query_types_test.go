package claudeagent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSDKControlInitializeResponseHooksApplied(t *testing.T) {
	tests := []struct {
		name string
		body string
		want *bool
	}{
		{
			name: "true when the CLI registered this initialize's hooks",
			body: `{"output_style": "default", "hooks_applied": true}`,
			want: boolPtr(true),
		},
		{
			name: "false when a repeated initialize's hooks were ignored",
			body: `{"output_style": "default", "hooks_applied": false}`,
			want: boolPtr(false),
		},
		{
			// Not the same as false: a CLI predating the field ignored
			// hooks on every repeated initialize without saying so.
			name: "nil when absent",
			body: `{"output_style": "default"}`,
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var resp SDKControlInitializeResponse
			require.NoError(t, json.Unmarshal([]byte(tc.body), &resp))
			assert.Equal(t, tc.want, resp.HooksApplied)
		})
	}
}

func TestSDKControlInitializeResponseHooksAppliedOmitEmpty(t *testing.T) {
	data, err := json.Marshal(SDKControlInitializeResponse{})
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "hooks_applied")
}
