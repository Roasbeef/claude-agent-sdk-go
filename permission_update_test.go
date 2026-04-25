package claudeagent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermissionUpdateJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		update     PermissionUpdate
		want       map[string]interface{}
		absentKeys []string
	}{
		{
			name: "addRules",
			update: PermissionUpdate{
				Type: PermissionUpdateTypeAddRules,
				Rules: []PermissionRule{
					{ToolName: "Bash", RuleContent: "git status"},
				},
				Behavior:    PermissionBehaviorAllow,
				Destination: PermissionDestinationProjectSettings,
			},
			want: map[string]interface{}{
				"type":        "addRules",
				"rules":       []interface{}{map[string]interface{}{"toolName": "Bash", "ruleContent": "git status"}},
				"behavior":    "allow",
				"destination": "projectSettings",
			},
			absentKeys: []string{"mode", "directories"},
		},
		{
			name: "replaceRules",
			update: PermissionUpdate{
				Type: PermissionUpdateTypeReplaceRules,
				Rules: []PermissionRule{
					{ToolName: "Read", RuleContent: "/tmp/*"},
				},
				Behavior:    PermissionBehaviorAsk,
				Destination: PermissionDestinationUserSettings,
			},
			want: map[string]interface{}{
				"type":        "replaceRules",
				"rules":       []interface{}{map[string]interface{}{"toolName": "Read", "ruleContent": "/tmp/*"}},
				"behavior":    "ask",
				"destination": "userSettings",
			},
			absentKeys: []string{"mode", "directories"},
		},
		{
			name: "removeRules",
			update: PermissionUpdate{
				Type: PermissionUpdateTypeRemoveRules,
				Rules: []PermissionRule{
					{ToolName: "Write", RuleContent: "/tmp/out"},
				},
				Behavior:    PermissionBehaviorDeny,
				Destination: PermissionDestinationLocalSettings,
			},
			want: map[string]interface{}{
				"type":        "removeRules",
				"rules":       []interface{}{map[string]interface{}{"toolName": "Write", "ruleContent": "/tmp/out"}},
				"behavior":    "deny",
				"destination": "localSettings",
			},
			absentKeys: []string{"mode", "directories"},
		},
		{
			name: "setMode",
			update: PermissionUpdate{
				Type:        PermissionUpdateTypeSetMode,
				Mode:        PermissionModePlan,
				Destination: PermissionDestinationSession,
			},
			want: map[string]interface{}{
				"type":        "setMode",
				"mode":        "plan",
				"destination": "session",
			},
			absentKeys: []string{"rules", "behavior", "directories"},
		},
		{
			name: "addDirectories",
			update: PermissionUpdate{
				Type:        PermissionUpdateTypeAddDirectories,
				Directories: []string{"/a", "/b"},
				Destination: PermissionDestinationCLIArg,
			},
			want: map[string]interface{}{
				"type":        "addDirectories",
				"directories": []interface{}{"/a", "/b"},
				"destination": "cliArg",
			},
			absentKeys: []string{"rules", "behavior", "mode"},
		},
		{
			name: "removeDirectories",
			update: PermissionUpdate{
				Type:        PermissionUpdateTypeRemoveDirectories,
				Directories: []string{"/c", "/d"},
				Destination: PermissionDestinationProjectSettings,
			},
			want: map[string]interface{}{
				"type":        "removeDirectories",
				"directories": []interface{}{"/c", "/d"},
				"destination": "projectSettings",
			},
			absentKeys: []string{"rules", "behavior", "mode"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.update)
			require.NoError(t, err)

			var got map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &got))

			for key, value := range tt.want {
				assert.Equal(t, value, got[key])
			}
			for _, key := range tt.absentKeys {
				assert.NotContains(t, got, key)
			}

			var decoded PermissionUpdate
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tt.update, decoded)
		})
	}
}

func TestPermissionRuleJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		rule       PermissionRule
		want       map[string]interface{}
		absentKeys []string
	}{
		{
			name: "withRuleContent",
			rule: PermissionRule{ToolName: "Bash", RuleContent: "git status"},
			want: map[string]interface{}{
				"toolName":    "Bash",
				"ruleContent": "git status",
			},
		},
		{
			name: "withoutRuleContent",
			rule: PermissionRule{ToolName: "Read"},
			want: map[string]interface{}{
				"toolName": "Read",
			},
			absentKeys: []string{"ruleContent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.rule)
			require.NoError(t, err)

			var got map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &got))

			for key, value := range tt.want {
				assert.Equal(t, value, got[key])
			}
			for _, key := range tt.absentKeys {
				assert.NotContains(t, got, key)
			}

			var decoded PermissionRule
			require.NoError(t, json.Unmarshal(data, &decoded))
			assert.Equal(t, tt.rule, decoded)
		})
	}
}
