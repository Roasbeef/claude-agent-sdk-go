package claudeagent

import (
	"context"
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

func TestSettingsManagedOrgFieldsJSON(t *testing.T) {
	t.Run("fallbackModel round-trips populated list", func(t *testing.T) {
		in := Settings{
			FallbackModel: []string{"opus", "sonnet", "default"},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, []interface{}{"opus", "sonnet", "default"}, got["fallbackModel"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, []string{"opus", "sonnet", "default"}, out.FallbackModel)
	})

	t.Run("fallbackModel nil and empty omitted", func(t *testing.T) {
		for _, in := range []Settings{
			{},
			{FallbackModel: []string{}},
		} {
			data, err := json.Marshal(in)
			require.NoError(t, err)

			var got map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &got))
			assert.NotContains(t, got, "fallbackModel")
		}
	})

	t.Run("fallbackModel single value round-trips", func(t *testing.T) {
		in := Settings{
			FallbackModel: []string{"default"},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, []string{"default"}, out.FallbackModel)
	})

	t.Run("policyHelper round-trips with all fields", func(t *testing.T) {
		timeout := 5000
		refresh := 60000
		in := Settings{
			PolicyHelper: &SettingsPolicyHelper{
				Path:              "/usr/local/bin/claude-policy",
				TimeoutMs:         &timeout,
				RefreshIntervalMs: &refresh,
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		ph, ok := got["policyHelper"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "/usr/local/bin/claude-policy", ph["path"])
		assert.Equal(t, float64(5000), ph["timeoutMs"])
		assert.Equal(t, float64(60000), ph["refreshIntervalMs"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.PolicyHelper)
		assert.Equal(t, "/usr/local/bin/claude-policy", out.PolicyHelper.Path)
		require.NotNil(t, out.PolicyHelper.TimeoutMs)
		assert.Equal(t, 5000, *out.PolicyHelper.TimeoutMs)
		require.NotNil(t, out.PolicyHelper.RefreshIntervalMs)
		assert.Equal(t, 60000, *out.PolicyHelper.RefreshIntervalMs)
	})

	t.Run("policyHelper omits optional ms fields when nil", func(t *testing.T) {
		in := Settings{
			PolicyHelper: &SettingsPolicyHelper{
				Path: "/usr/local/bin/claude-policy",
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		ph, ok := got["policyHelper"].(map[string]interface{})
		require.True(t, ok)
		assert.NotContains(t, ph, "timeoutMs")
		assert.NotContains(t, ph, "refreshIntervalMs")
	})

	t.Run("nil PolicyHelper omits key", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "policyHelper")
	})

	t.Run("boolean fields nil omits key", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		for _, k := range []string{
			"disableAgentView",
			"disableRemoteControl",
			"allowAllClaudeAiMcps",
			"isolatePeerMachines",
		} {
			assert.NotContains(t, got, k, "key %q must be omitted when nil", k)
		}
	})

	t.Run("boolean fields explicit false is emitted", func(t *testing.T) {
		f := false
		in := Settings{
			DisableAgentView:     &f,
			DisableRemoteControl: &f,
			AllowAllClaudeAiMcps: &f,
			IsolatePeerMachines:  &f,
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, false, got["disableAgentView"])
		assert.Equal(t, false, got["disableRemoteControl"])
		assert.Equal(t, false, got["allowAllClaudeAiMcps"])
		assert.Equal(t, false, got["isolatePeerMachines"])
	})

	t.Run("boolean fields explicit true round-trips", func(t *testing.T) {
		v := true
		in := Settings{
			DisableAgentView:     &v,
			DisableRemoteControl: &v,
			AllowAllClaudeAiMcps: &v,
			IsolatePeerMachines:  &v,
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.DisableAgentView)
		assert.Equal(t, true, *out.DisableAgentView)
		require.NotNil(t, out.DisableRemoteControl)
		assert.Equal(t, true, *out.DisableRemoteControl)
		require.NotNil(t, out.AllowAllClaudeAiMcps)
		assert.Equal(t, true, *out.AllowAllClaudeAiMcps)
		require.NotNil(t, out.IsolatePeerMachines)
		assert.Equal(t, true, *out.IsolatePeerMachines)
	})

	t.Run("string fields zero omits key", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		for _, k := range []string{
			"parentSettingsBehavior",
			"daemonColdStart",
			"disableDeepLinkRegistration",
			"defaultView",
			"claudeMd",
		} {
			assert.NotContains(t, got, k, "key %q must be omitted when zero", k)
		}
	})

	t.Run("string fields round-trip canonical values", func(t *testing.T) {
		in := Settings{
			ClaudeMD:                    "always run in dry-run mode",
			ParentSettingsBehavior:      "merge",
			DaemonColdStart:             "transient",
			DisableDeepLinkRegistration: "disable",
			DefaultView:                 "chat",
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, "always run in dry-run mode", out.ClaudeMD)
		assert.Equal(t, "merge", out.ParentSettingsBehavior)
		assert.Equal(t, "transient", out.DaemonColdStart)
		assert.Equal(t, "disable", out.DisableDeepLinkRegistration)
		assert.Equal(t, "chat", out.DefaultView)
	})

	t.Run("all managed-org fields round-trip together", func(t *testing.T) {
		v := true
		timeout := 5000
		in := Settings{
			PolicyHelper: &SettingsPolicyHelper{
				Path:      "/usr/local/bin/claude-policy",
				TimeoutMs: &timeout,
			},
			ClaudeMD:                    "managed memory",
			DisableAgentView:            &v,
			DisableRemoteControl:        &v,
			AllowAllClaudeAiMcps:        &v,
			ParentSettingsBehavior:      "first-wins",
			IsolatePeerMachines:         &v,
			DaemonColdStart:             "ask",
			DisableDeepLinkRegistration: "disable",
			DefaultView:                 "transcript",
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.PolicyHelper)
		assert.Equal(t, "/usr/local/bin/claude-policy", out.PolicyHelper.Path)
		require.NotNil(t, out.PolicyHelper.TimeoutMs)
		assert.Equal(t, 5000, *out.PolicyHelper.TimeoutMs)
		assert.Nil(t, out.PolicyHelper.RefreshIntervalMs)
		assert.Equal(t, "managed memory", out.ClaudeMD)
		require.NotNil(t, out.DisableAgentView)
		assert.Equal(t, true, *out.DisableAgentView)
		require.NotNil(t, out.DisableRemoteControl)
		assert.Equal(t, true, *out.DisableRemoteControl)
		require.NotNil(t, out.AllowAllClaudeAiMcps)
		assert.Equal(t, true, *out.AllowAllClaudeAiMcps)
		assert.Equal(t, "first-wins", out.ParentSettingsBehavior)
		require.NotNil(t, out.IsolatePeerMachines)
		assert.Equal(t, true, *out.IsolatePeerMachines)
		assert.Equal(t, "ask", out.DaemonColdStart)
		assert.Equal(t, "disable", out.DisableDeepLinkRegistration)
		assert.Equal(t, "transcript", out.DefaultView)
	})
}

func TestSettingsRequiredVersionsJSON(t *testing.T) {
	in := Settings{
		RequiredMinimumVersion: "0.3.168",
		RequiredMaximumVersion: "0.3.200",
	}
	data, err := json.Marshal(in)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "0.3.168", got["requiredMinimumVersion"])
	assert.Equal(t, "0.3.200", got["requiredMaximumVersion"])

	var out Settings
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, "0.3.168", out.RequiredMinimumVersion)
	assert.Equal(t, "0.3.200", out.RequiredMaximumVersion)

	data, err = json.Marshal(Settings{})
	require.NoError(t, err)
	empty := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &empty))
	assert.NotContains(t, empty, "requiredMinimumVersion")
	assert.NotContains(t, empty, "requiredMaximumVersion")
}

func TestSettingsParity0177JSON(t *testing.T) {
	in := Settings{
		EnforceAvailableModels:         boolPtr(true),
		DisableBundledSkills:           boolPtr(true),
		DisableArtifact:                boolPtr(false),
		WheelScrollAccelerationEnabled: boolPtr(true),
		FooterLinksRegexes: []SettingsFooterLinkRegex{
			{Type: "regex", Pattern: `PR #(?<id>\d+)`, URL: "https://example.com/pr/{id}", Label: "PR"},
		},
	}
	data, err := json.Marshal(in)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, true, got["enforceAvailableModels"])
	assert.Equal(t, true, got["disableBundledSkills"])
	assert.Equal(t, false, got["disableArtifact"])
	assert.Equal(t, true, got["wheelScrollAccelerationEnabled"])
	regexes, ok := got["footerLinksRegexes"].([]interface{})
	require.True(t, ok)
	require.Len(t, regexes, 1)
	entry := regexes[0].(map[string]interface{})
	assert.Equal(t, "regex", entry["type"])
	assert.Equal(t, "https://example.com/pr/{id}", entry["url"])

	var out Settings
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotNil(t, out.EnforceAvailableModels)
	assert.True(t, *out.EnforceAvailableModels)
	require.Len(t, out.FooterLinksRegexes, 1)
	assert.Equal(t, "PR", out.FooterLinksRegexes[0].Label)

	empty := map[string]interface{}{}
	data, err = json.Marshal(Settings{})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &empty))
	for _, k := range []string{
		"enforceAvailableModels", "disableBundledSkills", "disableArtifact",
		"footerLinksRegexes", "wheelScrollAccelerationEnabled",
	} {
		assert.NotContains(t, empty, k)
	}
}

func TestSettingsParity0185JSON(t *testing.T) {
	in := Settings{
		DisableClaudeAiConnectors: boolPtr(true),
		Attribution:               &SettingsAttribution{SessionURL: boolPtr(false)},
		Sandbox:                   &SettingsSandbox{AllowAppleEvents: boolPtr(true)},
	}
	data, err := json.Marshal(in)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, true, got["disableClaudeAiConnectors"])
	attr, ok := got["attribution"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, attr["sessionUrl"])
	sandbox, ok := got["sandbox"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, sandbox["allowAppleEvents"])

	var out Settings
	require.NoError(t, json.Unmarshal(data, &out))
	require.NotNil(t, out.DisableClaudeAiConnectors)
	assert.True(t, *out.DisableClaudeAiConnectors)
	require.NotNil(t, out.Attribution.SessionURL)
	assert.False(t, *out.Attribution.SessionURL)
	require.NotNil(t, out.Sandbox.AllowAppleEvents)
	assert.True(t, *out.Sandbox.AllowAppleEvents)

	empty := map[string]interface{}{}
	data, err = json.Marshal(Settings{})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &empty))
	assert.NotContains(t, empty, "disableClaudeAiConnectors")

	attrEmpty := map[string]interface{}{}
	data, err = json.Marshal(SettingsAttribution{})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &attrEmpty))
	assert.NotContains(t, attrEmpty, "sessionUrl")

	sandboxEmpty := map[string]interface{}{}
	data, err = json.Marshal(SettingsSandbox{})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &sandboxEmpty))
	assert.NotContains(t, sandboxEmpty, "allowAppleEvents")
}

func TestSettingsSwitchModelsOnFlagJSON(t *testing.T) {
	t.Run("nil omits key", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "switchModelsOnFlag")
	})

	for _, tc := range []struct {
		name string
		in   bool
	}{
		{name: "explicit false emits false", in: false},
		{name: "explicit true emits true", in: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := Settings{SwitchModelsOnFlag: &tc.in}
			data, err := json.Marshal(in)
			require.NoError(t, err)

			var got map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, tc.in, got["switchModelsOnFlag"])

			var out Settings
			require.NoError(t, json.Unmarshal(data, &out))
			require.NotNil(t, out.SwitchModelsOnFlag)
			assert.Equal(t, tc.in, *out.SwitchModelsOnFlag)
		})
	}
}

func TestSettingsForceLoginMethodGateway(t *testing.T) {
	in := Settings{ForceLoginMethod: "gateway"}
	data, err := json.Marshal(in)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, "gateway", got["forceLoginMethod"])

	var out Settings
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, "gateway", out.ForceLoginMethod)
}

func TestSettingsWorkflowsFieldsJSON(t *testing.T) {
	t.Run("nil and empty fields omitted", func(t *testing.T) {
		data, err := json.Marshal(Settings{
			PluginSuggestionMarketplaces: []string{},
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		for _, k := range []string{
			"disableWorkflows",
			"enableWorkflows",
			"workflowKeywordTriggerEnabled",
			"pluginSuggestionMarketplaces",
			"ultracode",
		} {
			assert.NotContains(t, got, k, "key %q must be omitted when nil or empty", k)
		}
	})

	t.Run("disableWorkflows true round-trips", func(t *testing.T) {
		v := true
		data, err := json.Marshal(Settings{DisableWorkflows: &v})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, true, got["disableWorkflows"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.DisableWorkflows)
		assert.True(t, *out.DisableWorkflows)
	})

	t.Run("enableWorkflows false round-trips", func(t *testing.T) {
		v := false
		data, err := json.Marshal(Settings{EnableWorkflows: &v})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, false, got["enableWorkflows"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.EnableWorkflows)
		assert.False(t, *out.EnableWorkflows)
	})

	t.Run("workflowKeywordTriggerEnabled true round-trips", func(t *testing.T) {
		v := true
		data, err := json.Marshal(Settings{WorkflowKeywordTriggerEnabled: &v})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, true, got["workflowKeywordTriggerEnabled"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.WorkflowKeywordTriggerEnabled)
		assert.True(t, *out.WorkflowKeywordTriggerEnabled)
	})

	t.Run("pluginSuggestionMarketplaces round-trips", func(t *testing.T) {
		data, err := json.Marshal(Settings{
			PluginSuggestionMarketplaces: []string{"a", "b"},
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, []interface{}{"a", "b"}, got["pluginSuggestionMarketplaces"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, []string{"a", "b"}, out.PluginSuggestionMarketplaces)
	})

	t.Run("ultracode true round-trips", func(t *testing.T) {
		v := true
		data, err := json.Marshal(Settings{Ultracode: &v})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, true, got["ultracode"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Ultracode)
		assert.True(t, *out.Ultracode)
	})

	t.Run("all fields round-trip together", func(t *testing.T) {
		enabled := true
		disabled := false
		in := Settings{
			DisableWorkflows:              &enabled,
			EnableWorkflows:               &disabled,
			WorkflowKeywordTriggerEnabled: &enabled,
			PluginSuggestionMarketplaces:  []string{"internal", "partner"},
			Ultracode:                     &enabled,
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.DisableWorkflows)
		assert.True(t, *out.DisableWorkflows)
		require.NotNil(t, out.EnableWorkflows)
		assert.False(t, *out.EnableWorkflows)
		require.NotNil(t, out.WorkflowKeywordTriggerEnabled)
		assert.True(t, *out.WorkflowKeywordTriggerEnabled)
		assert.Equal(t, []string{"internal", "partner"}, out.PluginSuggestionMarketplaces)
		require.NotNil(t, out.Ultracode)
		assert.True(t, *out.Ultracode)
	})
}

func TestSettingsSandboxFieldsJSON(t *testing.T) {
	t.Run("tlsTerminate round-trips with both paths", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Network: &SettingsSandboxNetwork{
					TLSTerminate: &SettingsSandboxTLSTerminate{
						CACertPath: "/etc/claude/ca.crt",
						CAKeyPath:  "/etc/claude/ca.key",
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sb, ok := got["sandbox"].(map[string]interface{})
		require.True(t, ok)
		nw, ok := sb["network"].(map[string]interface{})
		require.True(t, ok)
		tls, ok := nw["tlsTerminate"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "/etc/claude/ca.crt", tls["caCertPath"])
		assert.Equal(t, "/etc/claude/ca.key", tls["caKeyPath"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Sandbox)
		require.NotNil(t, out.Sandbox.Network)
		require.NotNil(t, out.Sandbox.Network.TLSTerminate)
		assert.Equal(t, "/etc/claude/ca.crt", out.Sandbox.Network.TLSTerminate.CACertPath)
		assert.Equal(t, "/etc/claude/ca.key", out.Sandbox.Network.TLSTerminate.CAKeyPath)
	})

	t.Run("tlsTerminate empty struct emits empty object", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Network: &SettingsSandboxNetwork{
					TLSTerminate: &SettingsSandboxTLSTerminate{},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sb := got["sandbox"].(map[string]interface{})
		nw := sb["network"].(map[string]interface{})
		tls, ok := nw["tlsTerminate"].(map[string]interface{})
		require.True(t, ok)
		assert.NotContains(t, tls, "caCertPath")
		assert.NotContains(t, tls, "caKeyPath")
	})

	t.Run("nil tlsTerminate omits key", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Network: &SettingsSandboxNetwork{},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sb := got["sandbox"].(map[string]interface{})
		nw := sb["network"].(map[string]interface{})
		assert.NotContains(t, nw, "tlsTerminate")
	})

	t.Run("bwrapPath and socatPath zero omits keys", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sb := got["sandbox"].(map[string]interface{})
		assert.NotContains(t, sb, "bwrapPath")
		assert.NotContains(t, sb, "socatPath")
	})

	t.Run("bwrapPath and socatPath emit set values", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				BwrapPath: "/usr/bin/bwrap",
				SocatPath: "/usr/bin/socat",
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sb := got["sandbox"].(map[string]interface{})
		assert.Equal(t, "/usr/bin/bwrap", sb["bwrapPath"])
		assert.Equal(t, "/usr/bin/socat", sb["socatPath"])
	})

	t.Run("all new sandbox fields round-trip together", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Network: &SettingsSandboxNetwork{
					AllowedDomains: []string{"example.com"},
					TLSTerminate: &SettingsSandboxTLSTerminate{
						CACertPath: "/etc/claude/ca.crt",
						CAKeyPath:  "/etc/claude/ca.key",
					},
				},
				BwrapPath: "/usr/bin/bwrap",
				SocatPath: "/usr/bin/socat",
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Sandbox)
		require.NotNil(t, out.Sandbox.Network)
		require.NotNil(t, out.Sandbox.Network.TLSTerminate)
		assert.Equal(t, []string{"example.com"}, out.Sandbox.Network.AllowedDomains)
		assert.Equal(t, "/etc/claude/ca.crt", out.Sandbox.Network.TLSTerminate.CACertPath)
		assert.Equal(t, "/etc/claude/ca.key", out.Sandbox.Network.TLSTerminate.CAKeyPath)
		assert.Equal(t, "/usr/bin/bwrap", out.Sandbox.BwrapPath)
		assert.Equal(t, "/usr/bin/socat", out.Sandbox.SocatPath)
	})
}

func TestSettingsWorktreeFieldsJSON(t *testing.T) {
	t.Run("baseRef and bgIsolation zero omits keys", func(t *testing.T) {
		in := Settings{
			Worktree: &SettingsWorktree{
				SymlinkDirectories: []string{"node_modules"},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		wt, ok := got["worktree"].(map[string]interface{})
		require.True(t, ok)
		assert.NotContains(t, wt, "baseRef")
		assert.NotContains(t, wt, "bgIsolation")
	})

	t.Run("baseRef set round-trip", func(t *testing.T) {
		in := Settings{
			Worktree: &SettingsWorktree{BaseRef: "head"},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		wt := got["worktree"].(map[string]interface{})
		assert.Equal(t, "head", wt["baseRef"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Worktree)
		assert.Equal(t, "head", out.Worktree.BaseRef)
	})

	t.Run("bgIsolation set round-trip", func(t *testing.T) {
		in := Settings{
			Worktree: &SettingsWorktree{BgIsolation: "none"},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		wt := got["worktree"].(map[string]interface{})
		assert.Equal(t, "none", wt["bgIsolation"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Worktree)
		assert.Equal(t, "none", out.Worktree.BgIsolation)
	})

	t.Run("all worktree fields round-trip together", func(t *testing.T) {
		in := Settings{
			Worktree: &SettingsWorktree{
				SymlinkDirectories: []string{"node_modules", ".cache"},
				SparsePaths:        []string{"src", "tests"},
				BaseRef:            "fresh",
				BgIsolation:        "worktree",
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Worktree)
		assert.Equal(t, []string{"node_modules", ".cache"}, out.Worktree.SymlinkDirectories)
		assert.Equal(t, []string{"src", "tests"}, out.Worktree.SparsePaths)
		assert.Equal(t, "fresh", out.Worktree.BaseRef)
		assert.Equal(t, "worktree", out.Worktree.BgIsolation)
	})
}

func TestSettingsCommandHideVimModeIndicatorJSON(t *testing.T) {
	t.Run("nil omits key", func(t *testing.T) {
		in := Settings{
			StatusLine: &SettingsCommand{
				Type:    "command",
				Command: "/usr/bin/statusline",
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sl, ok := got["statusLine"].(map[string]interface{})
		require.True(t, ok)
		assert.NotContains(t, sl, "hideVimModeIndicator")
	})

	t.Run("explicit false emits", func(t *testing.T) {
		hide := false
		in := Settings{
			StatusLine: &SettingsCommand{
				Type:                 "command",
				Command:              "/usr/bin/statusline",
				HideVimModeIndicator: &hide,
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sl := got["statusLine"].(map[string]interface{})
		assert.Equal(t, false, sl["hideVimModeIndicator"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.StatusLine)
		require.NotNil(t, out.StatusLine.HideVimModeIndicator)
		assert.False(t, *out.StatusLine.HideVimModeIndicator)
	})

	t.Run("explicit true round-trip", func(t *testing.T) {
		hide := true
		in := Settings{
			StatusLine: &SettingsCommand{
				Type:                 "command",
				Command:              "/usr/bin/statusline",
				HideVimModeIndicator: &hide,
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.StatusLine)
		require.NotNil(t, out.StatusLine.HideVimModeIndicator)
		assert.True(t, *out.StatusLine.HideVimModeIndicator)
	})
}

func TestSettingsMarketplaceSourceVariants(t *testing.T) {
	t.Run("skills-dir bare tag round-trip", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"my-skills": {
					Source: SettingsMarketplaceSource{
						"source": string(SettingsMarketplaceSourceSkillsDir),
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["my-skills"].Source
		assert.Equal(t, "skills-dir", got["source"])
		assert.Len(t, got, 1)
	})

	t.Run("unsupported bare tag round-trip", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"legacy": {
					Source: SettingsMarketplaceSource{
						"source": string(SettingsMarketplaceSourceUnsupported),
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["legacy"].Source
		assert.Equal(t, "unsupported", got["source"])
		assert.Len(t, got, 1)
	})

	t.Run("github variant still round-trips alongside", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"upstream": {
					Source: SettingsMarketplaceSource{
						"source": string(SettingsMarketplaceSourceGithub),
						"repo":   "anthropics/skills",
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["upstream"].Source
		assert.Equal(t, "github", got["source"])
		assert.Equal(t, "anthropics/skills", got["repo"])
	})

	t.Run("constants resolve to documented strings", func(t *testing.T) {
		assert.Equal(t, "skills-dir", string(SettingsMarketplaceSourceSkillsDir))
		assert.Equal(t, "unsupported", string(SettingsMarketplaceSourceUnsupported))
	})

	t.Run("github source with skipLfs true round-trips", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"upstream": {
					Source: SettingsMarketplaceSource{
						"source":  string(SettingsMarketplaceSourceGithub),
						"repo":    "anthropics/skills",
						"skipLfs": true,
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["upstream"].Source
		assert.Equal(t, "github", got["source"])
		assert.Equal(t, "anthropics/skills", got["repo"])
		assert.Equal(t, true, got["skipLfs"])
	})

	t.Run("git source with skipLfs false round-trips", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"mirror": {
					Source: SettingsMarketplaceSource{
						"source":  string(SettingsMarketplaceSourceGit),
						"url":     "https://example.invalid/marketplace.git",
						"skipLfs": false,
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["mirror"].Source
		assert.Equal(t, "git", got["source"])
		assert.Equal(t, "https://example.invalid/marketplace.git", got["url"])
		assert.Equal(t, false, got["skipLfs"])
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

func TestWithOnUserDialog(t *testing.T) {
	t.Run("builder installs callback", func(t *testing.T) {
		opts := NewOptions()
		require.Nil(t, opts.OnUserDialog)

		called := false
		fn := func(ctx context.Context, req UserDialogRequest) (UserDialogResult, error) {
			called = true
			return UserDialogResult{Behavior: UserDialogBehaviorCancelled}, nil
		}

		WithOnUserDialog(fn)(opts)
		require.NotNil(t, opts.OnUserDialog)

		_, err := opts.OnUserDialog(context.Background(), UserDialogRequest{DialogKind: "k"})
		require.NoError(t, err)
		assert.True(t, called)
	})
}

func TestUserDialogRequest_ToolUseIDOmitempty(t *testing.T) {
	t.Run("missing tool_use_id is absent on the wire", func(t *testing.T) {
		req := UserDialogRequest{
			DialogKind: "approve_edit",
			Payload:    map[string]interface{}{"path": "/tmp/x"},
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "approve_edit", got["dialogKind"])
		assert.Equal(t, map[string]interface{}{"path": "/tmp/x"}, got["payload"])
		_, hasToolUseID := got["toolUseID"]
		assert.False(t, hasToolUseID)
	})

	t.Run("present tool_use_id round-trips", func(t *testing.T) {
		req := UserDialogRequest{
			DialogKind: "approve_edit",
			Payload:    map[string]interface{}{},
			ToolUseID:  "tu_42",
		}
		data, err := json.Marshal(req)
		require.NoError(t, err)

		var got UserDialogRequest
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "tu_42", got.ToolUseID)
	})
}

func TestPluginConfigJSON(t *testing.T) {
	t.Run("marshal includes skipMcpDiscovery when set", func(t *testing.T) {
		data, err := json.Marshal(PluginConfig{
			Type:             "local",
			Path:             "./plugins/foo",
			SkipMcpDiscovery: true,
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "local", got["type"])
		assert.Equal(t, "./plugins/foo", got["path"])
		assert.Equal(t, true, got["skipMcpDiscovery"])
	})

	t.Run("omits skipMcpDiscovery when false", func(t *testing.T) {
		data, err := json.Marshal(PluginConfig{Type: "local", Path: "./plugins/bar"})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "skipMcpDiscovery")
	})

	t.Run("WithPlugins round-trips the field", func(t *testing.T) {
		opts := NewOptions()
		WithPlugins([]PluginConfig{
			{Type: "local", Path: "./p", SkipMcpDiscovery: true},
		})(opts)
		require.Len(t, opts.Plugins, 1)
		assert.True(t, opts.Plugins[0].SkipMcpDiscovery)
	})
}
