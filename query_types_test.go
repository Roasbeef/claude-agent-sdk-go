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

// TestMcpServerStatusSource covers the source field v0.3.278 adds to the
// mcp_status rows (sdk.d.ts L1197). It is the reporting-side twin of the
// provenance carried on permission and hook payloads.
func TestMcpServerStatusSource(t *testing.T) {
	t.Run("sdk server", func(t *testing.T) {
		var got McpServerStatus
		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "calculator",
			"status": "connected",
			"serverInfo": {"name": "calculator", "version": "1.0.0"},
			"source": "sdk"
		}`), &got))

		assert.Equal(t, MCPServerSourceSDK, got.Source)
		assert.True(t, got.IsSDK())
	})

	t.Run("configured server sharing the name is not sdk", func(t *testing.T) {
		var got McpServerStatus
		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "calculator",
			"status": "connected",
			"source": "project"
		}`), &got))

		assert.Equal(t, MCPServerSourceProject, got.Source)
		assert.False(t, got.IsSDK())
	})

	// An older CLI sends no source at all. Unknown must not read as SDK.
	t.Run("absent source is unknown, not sdk", func(t *testing.T) {
		var got McpServerStatus
		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "calculator",
			"status": "connected"
		}`), &got))

		assert.Empty(t, got.Source)
		assert.False(t, got.IsSDK())
	})

	t.Run("unknown source from a newer CLI is not sdk", func(t *testing.T) {
		var got McpServerStatus
		require.NoError(t, json.Unmarshal([]byte(`{
			"name": "future",
			"status": "connected",
			"source": "some-source-from-a-newer-cli"
		}`), &got))

		assert.Equal(t, MCPServerSource("some-source-from-a-newer-cli"), got.Source)
		assert.False(t, got.IsSDK())
	})

	// The field rides reload_plugins rows too, since they are the same type.
	t.Run("reload_plugins rows carry it", func(t *testing.T) {
		var got SDKControlReloadPluginsResponse
		require.NoError(t, json.Unmarshal([]byte(`{
			"commands": [],
			"agents": [],
			"plugins": [],
			"mcpServers": [{"name": "acme", "status": "connected", "source": "plugin"}],
			"error_count": 0
		}`), &got))

		require.Len(t, got.McpServers, 1)
		assert.Equal(t, MCPServerSourcePlugin, got.McpServers[0].Source)
		assert.False(t, got.McpServers[0].IsSDK())
	})
}

// TestSlashCommandBuiltin covers the builtin marker v0.3.278 adds to command
// rows (sdk.d.ts L8938).
func TestSlashCommandBuiltin(t *testing.T) {
	var got SlashCommand
	require.NoError(t, json.Unmarshal([]byte(`{
		"name": "usage",
		"description": "show usage",
		"argumentHint": "",
		"aliases": ["cost", "stats"],
		"builtin": true
	}`), &got))
	assert.True(t, got.Builtin)
	assert.Equal(t, []string{"cost", "stats"}, got.Aliases)

	// Absent marker (user/project/plugin command) reads false and is omitted
	// on the way back out.
	var custom SlashCommand
	require.NoError(t, json.Unmarshal([]byte(`{"name":"deploy","description":"","argumentHint":""}`), &custom))
	assert.False(t, custom.Builtin)

	data, err := json.Marshal(custom)
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "builtin")
}

// TestHooksListingPolicyUnreadable covers the fail-closed marker v0.3.278 adds
// to the /hooks policy block (sdk.d.ts L3957).
func TestHooksListingPolicyUnreadable(t *testing.T) {
	var got HooksListingPolicy
	require.NoError(t, json.Unmarshal([]byte(`{
		"disabledByPolicy": false,
		"managedOnly": false,
		"pluginOnly": false,
		"allDisabled": false,
		"policyHookCount": 0,
		"policyUnreadable": true
	}`), &got))
	assert.True(t, got.PolicyUnreadable)

	// Readable managed settings omit the field; it must not marshal back.
	data, err := json.Marshal(HooksListingPolicy{})
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "policyUnreadable")
}
