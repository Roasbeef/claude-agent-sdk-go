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

	t.Run("v0.3.215 fields round-trip and omit when zero", func(t *testing.T) {
		empty, err := json.Marshal(Settings{})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(empty, &got))
		assert.NotContains(t, got, "processWrapper")
		assert.NotContains(t, got, "feedbackDrafts")
		assert.NotContains(t, got, "vimInsertModeRemaps")

		in := Settings{
			ProcessWrapper:      "/usr/bin/corp-launch --",
			FeedbackDrafts:      "quiet",
			VimInsertModeRemaps: map[string]interface{}{"jj": "<Esc>"},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, "/usr/bin/corp-launch --", out.ProcessWrapper)
		assert.Equal(t, "quiet", out.FeedbackDrafts)
		assert.Equal(t, "<Esc>", out.VimInsertModeRemaps["jj"])
	})

	t.Run("v0.3.220 fields round-trip and omit when zero", func(t *testing.T) {
		empty, err := json.Marshal(Settings{})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(empty, &got))
		assert.NotContains(t, got, "workflowSizeGuideline")
		assert.NotContains(t, got, "emojiCompletionEnabled")
		assert.NotContains(t, got, "precomputeCompactionEnabled")
		assert.NotContains(t, got, "voiceEnabled")

		enabled := true
		in := Settings{
			WorkflowSizeGuideline:       "large",
			EmojiCompletionEnabled:      &enabled,
			PrecomputeCompactionEnabled: &enabled,
			VoiceEnabled:                &enabled,
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, "large", out.WorkflowSizeGuideline)
		require.NotNil(t, out.EmojiCompletionEnabled)
		assert.True(t, *out.EmojiCompletionEnabled)
		require.NotNil(t, out.PrecomputeCompactionEnabled)
		assert.True(t, *out.PrecomputeCompactionEnabled)
		require.NotNil(t, out.VoiceEnabled)
		assert.True(t, *out.VoiceEnabled)
	})

	t.Run("v0.3.226 fields round-trip and omit when zero", func(t *testing.T) {
		empty, err := json.Marshal(Settings{})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(empty, &got))
		assert.NotContains(t, got, "dialogExpiry")
		assert.NotContains(t, got, "crossSessionInbound")

		in := Settings{
			DialogExpiry:        "never",
			CrossSessionInbound: "hold",
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "never", got["dialogExpiry"])
		assert.Equal(t, "hold", got["crossSessionInbound"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, "never", out.DialogExpiry)
		assert.Equal(t, "hold", out.CrossSessionInbound)
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

	t.Run("network strictAllowlist and filesystem disabled round-trip", func(t *testing.T) {
		trueVal := true
		in := Settings{
			Sandbox: &SettingsSandbox{
				Network:    &SettingsSandboxNetwork{StrictAllowlist: &trueVal},
				Filesystem: &SettingsSandboxFilesystem{Disabled: &trueVal},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"strictAllowlist":true`)
		assert.Contains(t, string(data), `"disabled":true`)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Sandbox.Network.StrictAllowlist)
		assert.True(t, *out.Sandbox.Network.StrictAllowlist)
		require.NotNil(t, out.Sandbox.Filesystem.Disabled)
		assert.True(t, *out.Sandbox.Filesystem.Disabled)

		// Both omitempty when unset.
		bare, err := json.Marshal(Settings{Sandbox: &SettingsSandbox{
			Network: &SettingsSandboxNetwork{}, Filesystem: &SettingsSandboxFilesystem{},
		}})
		require.NoError(t, err)
		assert.NotContains(t, string(bare), "strictAllowlist")
		assert.NotContains(t, string(bare), "disabled")
	})

	t.Run("credentials round-trip files and envVars", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					Files: []SettingsSandboxCredentialFile{
						{Path: "~/.aws/credentials", Mode: "deny"},
					},
					EnvVars: []SettingsSandboxCredentialEnvVar{
						{Name: "AWS_SECRET_ACCESS_KEY", Mode: "deny"},
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sb := got["sandbox"].(map[string]interface{})
		creds, ok := sb["credentials"].(map[string]interface{})
		require.True(t, ok)
		files := creds["files"].([]interface{})
		require.Len(t, files, 1)
		assert.Equal(t, "~/.aws/credentials", files[0].(map[string]interface{})["path"])
		assert.Equal(t, "deny", files[0].(map[string]interface{})["mode"])
		envVars := creds["envVars"].([]interface{})
		require.Len(t, envVars, 1)
		assert.Equal(t, "AWS_SECRET_ACCESS_KEY", envVars[0].(map[string]interface{})["name"])
		assert.Equal(t, "deny", envVars[0].(map[string]interface{})["mode"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Sandbox.Credentials)
		require.Len(t, out.Sandbox.Credentials.Files, 1)
		assert.Equal(t, "~/.aws/credentials", out.Sandbox.Credentials.Files[0].Path)
		require.Len(t, out.Sandbox.Credentials.EnvVars, 1)
		assert.Equal(t, "AWS_SECRET_ACCESS_KEY", out.Sandbox.Credentials.EnvVars[0].Name)
	})

	t.Run("credentials mask mode with injectHosts and allowPlaintextInject", func(t *testing.T) {
		allowPlaintext := true
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					EnvVars: []SettingsSandboxCredentialEnvVar{
						{
							Name:        "STRIPE_KEY",
							Mode:        "mask",
							InjectHosts: []string{"api.stripe.com"},
						},
					},
					AllowPlaintextInject: &allowPlaintext,
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		creds := got["sandbox"].(map[string]interface{})["credentials"].(map[string]interface{})
		assert.Equal(t, true, creds["allowPlaintextInject"])
		env := creds["envVars"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, "mask", env["mode"])
		assert.Equal(t, []interface{}{"api.stripe.com"}, env["injectHosts"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.Sandbox.Credentials.AllowPlaintextInject)
		assert.True(t, *out.Sandbox.Credentials.AllowPlaintextInject)
		require.Len(t, out.Sandbox.Credentials.EnvVars, 1)
		assert.Equal(t, "mask", out.Sandbox.Credentials.EnvVars[0].Mode)
		assert.Equal(t, []string{"api.stripe.com"}, out.Sandbox.Credentials.EnvVars[0].InjectHosts)
	})

	t.Run("deny envVar omits injectHosts and nil allowPlaintextInject omits key", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					EnvVars: []SettingsSandboxCredentialEnvVar{
						{Name: "AWS_SECRET_ACCESS_KEY", Mode: "deny"},
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		creds := got["sandbox"].(map[string]interface{})["credentials"].(map[string]interface{})
		assert.NotContains(t, creds, "allowPlaintextInject")
		env := creds["envVars"].([]interface{})[0].(map[string]interface{})
		assert.NotContains(t, env, "injectHosts")
	})

	t.Run("credential file mask with extract round-trips", func(t *testing.T) {
		maskDuplicates := true
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					Files: []SettingsSandboxCredentialFile{
						{
							Path:             "~/.netrc",
							Mode:             "mask",
							Extract:          `password\s+(\S+)`,
							OnExtractNoMatch: "deny",
							MaskDuplicates:   &maskDuplicates,
							InjectHosts:      []string{"api.example.com"},
						},
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		creds := got["sandbox"].(map[string]interface{})["credentials"].(map[string]interface{})
		file := creds["files"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, "mask", file["mode"])
		assert.Equal(t, `password\s+(\S+)`, file["extract"])
		assert.Equal(t, "deny", file["onExtractNoMatch"])
		assert.Equal(t, true, file["maskDuplicates"])
		assert.Equal(t, []interface{}{"api.example.com"}, file["injectHosts"])
		assert.NotContains(t, file, "decode")
		assert.NotContains(t, file, "maskClaims")

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.Len(t, out.Sandbox.Credentials.Files, 1)
		gotFile := out.Sandbox.Credentials.Files[0]
		assert.Equal(t, "mask", gotFile.Mode)
		assert.Equal(t, `password\s+(\S+)`, gotFile.Extract)
		assert.Equal(t, "deny", gotFile.OnExtractNoMatch)
		require.NotNil(t, gotFile.MaskDuplicates)
		assert.True(t, *gotFile.MaskDuplicates)
		assert.Equal(t, []string{"api.example.com"}, gotFile.InjectHosts)
	})

	t.Run("credential jwt decode round-trips on files and envVars", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					Files: []SettingsSandboxCredentialFile{
						{
							Path:       "~/.config/token.json",
							Mode:       "mask",
							Decode:     "jwt",
							MaskClaims: []string{"sub", "email"},
						},
					},
					EnvVars: []SettingsSandboxCredentialEnvVar{
						{
							Name:             "SERVICE_JWT",
							Mode:             "mask",
							Decode:           "jwt",
							MaskClaims:       []string{"sub"},
							OnExtractNoMatch: "warn",
						},
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		creds := got["sandbox"].(map[string]interface{})["credentials"].(map[string]interface{})
		file := creds["files"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, "jwt", file["decode"])
		assert.Equal(t, []interface{}{"sub", "email"}, file["maskClaims"])
		env := creds["envVars"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, "jwt", env["decode"])
		assert.Equal(t, []interface{}{"sub"}, env["maskClaims"])
		assert.Equal(t, "warn", env["onExtractNoMatch"])
		assert.NotContains(t, env, "extract")

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, []string{"sub", "email"}, out.Sandbox.Credentials.Files[0].MaskClaims)
		assert.Equal(t, "jwt", out.Sandbox.Credentials.EnvVars[0].Decode)
		assert.Equal(t, []string{"sub"}, out.Sandbox.Credentials.EnvVars[0].MaskClaims)
	})

	t.Run("deny entries omit every masking knob", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					Files: []SettingsSandboxCredentialFile{
						{Path: "~/.ssh/id_ed25519", Mode: "deny"},
					},
					EnvVars: []SettingsSandboxCredentialEnvVar{
						{Name: "AWS_SECRET_ACCESS_KEY", Mode: "deny"},
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		creds := got["sandbox"].(map[string]interface{})["credentials"].(map[string]interface{})
		for _, entry := range []map[string]interface{}{
			creds["files"].([]interface{})[0].(map[string]interface{}),
			creds["envVars"].([]interface{})[0].(map[string]interface{}),
		} {
			for _, k := range []string{
				"extract", "onExtractNoMatch", "decode", "maskClaims",
				"maskDuplicates", "injectHosts",
			} {
				assert.NotContains(t, entry, k)
			}
		}
	})

	t.Run("credentials awsPairs and sigv4 round-trip", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					EnvVars: []SettingsSandboxCredentialEnvVar{
						{Name: "CORP_AWS_KEY_ID", Mode: "mask"},
						{Name: "CORP_AWS_SECRET", Mode: "mask"},
					},
					AWSPairs: []SettingsSandboxCredentialAWSPair{
						{
							AccessKeyIDVar:     "CORP_AWS_KEY_ID",
							SecretAccessKeyVar: "CORP_AWS_SECRET",
						},
						{
							AccessKeyIDVar:     "TMP_KEY_ID",
							SecretAccessKeyVar: "TMP_SECRET",
							SessionTokenVar:    "TMP_SESSION_TOKEN",
						},
					},
					SigV4: &SettingsSandboxCredentialSigV4{
						Streaming: "passthrough",
						Presigned: "deny",
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		creds := got["sandbox"].(map[string]interface{})["credentials"].(map[string]interface{})
		pairs := creds["awsPairs"].([]interface{})
		require.Len(t, pairs, 2)
		first := pairs[0].(map[string]interface{})
		assert.Equal(t, "CORP_AWS_KEY_ID", first["accessKeyIdVar"])
		assert.Equal(t, "CORP_AWS_SECRET", first["secretAccessKeyVar"])
		assert.NotContains(t, first, "sessionTokenVar")
		assert.Equal(t, "TMP_SESSION_TOKEN", pairs[1].(map[string]interface{})["sessionTokenVar"])
		sigv4 := creds["sigv4"].(map[string]interface{})
		assert.Equal(t, "passthrough", sigv4["streaming"])
		assert.Equal(t, "deny", sigv4["presigned"])
		assert.NotContains(t, sigv4, "sigv4a")

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.Len(t, out.Sandbox.Credentials.AWSPairs, 2)
		assert.Equal(t, "CORP_AWS_KEY_ID", out.Sandbox.Credentials.AWSPairs[0].AccessKeyIDVar)
		assert.Empty(t, out.Sandbox.Credentials.AWSPairs[0].SessionTokenVar)
		assert.Equal(t, "TMP_SESSION_TOKEN", out.Sandbox.Credentials.AWSPairs[1].SessionTokenVar)
		require.NotNil(t, out.Sandbox.Credentials.SigV4)
		assert.Equal(t, "passthrough", out.Sandbox.Credentials.SigV4.Streaming)
		assert.Equal(t, "deny", out.Sandbox.Credentials.SigV4.Presigned)
		assert.Empty(t, out.Sandbox.Credentials.SigV4.SigV4A)
	})

	t.Run("credentials without awsPairs or sigv4 omit both keys", func(t *testing.T) {
		in := Settings{
			Sandbox: &SettingsSandbox{
				Credentials: &SettingsSandboxCredentials{
					EnvVars: []SettingsSandboxCredentialEnvVar{
						{Name: "AWS_SECRET_ACCESS_KEY", Mode: "mask"},
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		creds := got["sandbox"].(map[string]interface{})["credentials"].(map[string]interface{})
		assert.NotContains(t, creds, "awsPairs")
		assert.NotContains(t, creds, "sigv4")
	})

	t.Run("nil credentials omits key", func(t *testing.T) {
		data, err := json.Marshal(Settings{Sandbox: &SettingsSandbox{}})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		sb := got["sandbox"].(map[string]interface{})
		assert.NotContains(t, sb, "credentials")
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

	t.Run("archive source round-trips with url and sha256", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"vendor-bundle": {
					Source: SettingsMarketplaceSource{
						"source": string(SettingsMarketplaceSourceArchive),
						"url":    "https://example.com/plugins/bundle.zip",
						"sha256": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["vendor-bundle"].Source
		assert.Equal(t, "archive", got["source"])
		assert.Equal(t, "https://example.com/plugins/bundle.zip", got["url"])
		assert.Equal(t, "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", got["sha256"])
	})

	t.Run("archive source round-trips without sha256", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"vendor-bundle": {
					Source: SettingsMarketplaceSource{
						"source": string(SettingsMarketplaceSourceArchive),
						"url":    "https://example.com/plugins/bundle.zip",
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["vendor-bundle"].Source
		assert.Len(t, got, 2)
		assert.NotContains(t, got, "sha256")
	})

	t.Run("command source round-trips with timeout and mode", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"internal-export": {
					Source: SettingsMarketplaceSource{
						"source":  string(SettingsMarketplaceSourceCommand),
						"command": "/opt/corp/bin/export-plugin --print-path",
						"timeout": 120,
						"mode":    "link",
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["internal-export"].Source
		assert.Equal(t, "command", got["source"])
		assert.Equal(t, "/opt/corp/bin/export-plugin --print-path", got["command"])
		assert.Equal(t, float64(120), got["timeout"])
		assert.Equal(t, "link", got["mode"])
	})

	t.Run("command source round-trips bare", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"internal-export": {
					Source: SettingsMarketplaceSource{
						"source":  string(SettingsMarketplaceSourceCommand),
						"command": "echo /srv/plugins/corp",
					},
				},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		got := out.ExtraKnownMarketplaces["internal-export"].Source
		assert.Len(t, got, 2)
		assert.NotContains(t, got, "timeout")
		assert.NotContains(t, got, "mode")
	})

	t.Run("unsupported source carries an error string", func(t *testing.T) {
		in := Settings{
			ExtraKnownMarketplaces: map[string]SettingsMarketplace{
				"legacy": {
					Source: SettingsMarketplaceSource{
						"source": string(SettingsMarketplaceSourceUnsupported),
						"error":  "archive digest mismatch",
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
		assert.Equal(t, "archive digest mismatch", got["error"])
	})

	t.Run("github owner wildcard round-trips in policy lists", func(t *testing.T) {
		in := Settings{
			StrictKnownMarketplaces: []SettingsMarketplaceSource{
				{"source": string(SettingsMarketplaceSourceGithub), "repo": "anthropics/*"},
			},
			BlockedMarketplaces: []SettingsMarketplaceSource{
				{"source": string(SettingsMarketplaceSourceGithub), "repo": "untrusted/*"},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.Len(t, out.StrictKnownMarketplaces, 1)
		assert.Equal(t, "anthropics/*", out.StrictKnownMarketplaces[0]["repo"])
		require.Len(t, out.BlockedMarketplaces, 1)
		assert.Equal(t, "untrusted/*", out.BlockedMarketplaces[0]["repo"])
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

func TestBaseHookInputPromptID(t *testing.T) {
	t.Run("unmarshal populates prompt_id", func(t *testing.T) {
		var input PreToolUseInput
		require.NoError(t, json.Unmarshal(
			[]byte(`{"session_id":"s","transcript_path":"/t","cwd":"/c","prompt_id":"p-123","tool_name":"Read"}`),
			&input,
		))
		assert.Equal(t, "p-123", input.PromptID)
		assert.Equal(t, "p-123", input.Base().PromptID)
	})

	t.Run("empty omitted on marshal", func(t *testing.T) {
		data, err := json.Marshal(PreToolUseInput{
			BaseHookInput: BaseHookInput{SessionID: "s", TranscriptPath: "/t", Cwd: "/c"},
			ToolName:      "Read",
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "prompt_id")
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

func TestWithResumeDropsTurn(t *testing.T) {
	t.Run("builder sets the fork-point guard", func(t *testing.T) {
		opts := NewOptions()
		require.Empty(t, opts.SessionOptions.ResumeDropsTurn)

		WithResumeSessionAt("chain-uuid-1")(opts)
		WithResumeDropsTurn("prompt-uuid-2")(opts)

		assert.Equal(t, "chain-uuid-1", opts.SessionOptions.ResumeSessionAt)
		assert.Equal(t, "prompt-uuid-2", opts.SessionOptions.ResumeDropsTurn)
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

func TestSettingsParityV0195FieldsJSON(t *testing.T) {
	t.Run("explicit false is emitted", func(t *testing.T) {
		f := false
		data, err := json.Marshal(Settings{
			RespondToBashCommands: &f,
			DisableSideloadFlags:  &f,
		})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, false, got["respondToBashCommands"])
		assert.Equal(t, false, got["disableSideloadFlags"])
	})

	t.Run("nil omits keys", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "respondToBashCommands")
		assert.NotContains(t, got, "disableSideloadFlags")
	})

	t.Run("teammateMode iterm2 round-trips", func(t *testing.T) {
		data, err := json.Marshal(Settings{TeammateMode: "iterm2"})
		require.NoError(t, err)
		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, "iterm2", out.TeammateMode)
	})
}

func TestSettingsParityV0201FieldsJSON(t *testing.T) {
	t.Run("enableArtifact explicit false is emitted", func(t *testing.T) {
		f := false
		data, err := json.Marshal(Settings{EnableArtifact: &f})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, false, got["enableArtifact"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.EnableArtifact)
		assert.False(t, *out.EnableArtifact)
	})

	t.Run("askUserQuestionTimeout round-trips", func(t *testing.T) {
		data, err := json.Marshal(Settings{AskUserQuestionTimeout: "5m"})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "5m", got["askUserQuestionTimeout"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, "5m", out.AskUserQuestionTimeout)
	})

	t.Run("nil/empty omits keys", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)
		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "enableArtifact")
		assert.NotContains(t, got, "askUserQuestionTimeout")
	})
}

func TestSettingsDisableCommandPluginSourcesJSON(t *testing.T) {
	t.Run("explicit values are emitted", func(t *testing.T) {
		for _, v := range []bool{true, false} {
			data, err := json.Marshal(Settings{
				DisableCommandPluginSources: &v,
			})
			require.NoError(t, err)

			var got map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &got))
			assert.Equal(t, v, got["disableCommandPluginSources"])
		}
	})

	// Unset is not the same as false: it defers to allowManagedHooksOnly, so
	// the key has to stay off the wire rather than serialize as false.
	t.Run("nil omits the key", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "disableCommandPluginSources")
	})

	t.Run("round-trips", func(t *testing.T) {
		tr := true
		data, err := json.Marshal(Settings{DisableCommandPluginSources: &tr})
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		require.NotNil(t, out.DisableCommandPluginSources)
		assert.True(t, *out.DisableCommandPluginSources)
	})
}

// The alias keys have to serialize under their own spelling and stay
// independent of the canonical keys, since the CLI warns and drops the alias
// when both appear in one file.
func TestSettingsMarketplaceAliasesJSON(t *testing.T) {
	t.Run("aliases serialize under their own keys", func(t *testing.T) {
		data, err := json.Marshal(Settings{
			AdditionalMarketplaces: map[string]SettingsMarketplace{
				"vendor": {
					Source: SettingsMarketplaceSource{
						"source":  string(SettingsMarketplaceSourceNPM),
						"package": "@vendor/plugins",
					},
				},
			},
			AllowedMarketplaces: []SettingsMarketplaceSource{
				{
					"source":      string(SettingsMarketplaceSourceHostPattern),
					"hostPattern": "^github\\.corp\\.example$",
				},
			},
		})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Contains(t, got, "additionalMarketplaces")
		assert.Contains(t, got, "allowedMarketplaces")
		assert.NotContains(t, got, "extraKnownMarketplaces")
		assert.NotContains(t, got, "strictKnownMarketplaces")
	})

	t.Run("aliases round-trip", func(t *testing.T) {
		in := Settings{
			AdditionalMarketplaces: map[string]SettingsMarketplace{
				"vendor": {
					Source: SettingsMarketplaceSource{
						"source": string(SettingsMarketplaceSourceGithub),
						"repo":   "vendor/plugins",
						"ref":    "v1.0.0",
					},
					InstallLocation: "/opt/marketplaces/vendor",
				},
			},
			AllowedMarketplaces: []SettingsMarketplaceSource{
				{"source": string(SettingsMarketplaceSourceGithub), "repo": "vendor/*"},
			},
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))

		vendor := out.AdditionalMarketplaces["vendor"]
		assert.Equal(t, "github", vendor.Source["source"])
		assert.Equal(t, "vendor/plugins", vendor.Source["repo"])
		assert.Equal(t, "/opt/marketplaces/vendor", vendor.InstallLocation)

		require.Len(t, out.AllowedMarketplaces, 1)
		assert.Equal(t, "vendor/*", out.AllowedMarketplaces[0]["repo"])
	})

	// Both spellings decode side by side. The CLI is what resolves the
	// conflict, so the SDK must not silently fold one into the other.
	t.Run("canonical and alias decode independently", func(t *testing.T) {
		var out Settings
		require.NoError(t, json.Unmarshal([]byte(`{
			"extraKnownMarketplaces": {
				"canonical": {"source": {"source": "npm", "package": "@a/x"}}
			},
			"additionalMarketplaces": {
				"aliased": {"source": {"source": "npm", "package": "@b/y"}}
			},
			"strictKnownMarketplaces": [{"source": "skills-dir"}],
			"allowedMarketplaces": [{"source": "pathPattern", "pathPattern": "^/opt/"}]
		}`), &out))

		assert.Contains(t, out.ExtraKnownMarketplaces, "canonical")
		assert.Contains(t, out.AdditionalMarketplaces, "aliased")
		assert.Len(t, out.StrictKnownMarketplaces, 1)
		assert.Len(t, out.AllowedMarketplaces, 1)
	})

	t.Run("nil omits both keys", func(t *testing.T) {
		data, err := json.Marshal(Settings{})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "additionalMarketplaces")
		assert.NotContains(t, got, "allowedMarketplaces")
	})
}

func TestSettingsForceLoginGatewayURLJSON(t *testing.T) {
	t.Run("round-trips alongside forceLoginMethod", func(t *testing.T) {
		in := Settings{
			ForceLoginMethod:     "gateway",
			ForceLoginGatewayURL: "https://gateway.corp.example",
		}
		data, err := json.Marshal(in)
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.Equal(t, "https://gateway.corp.example", got["forceLoginGatewayUrl"])

		var out Settings
		require.NoError(t, json.Unmarshal(data, &out))
		assert.Equal(t, in.ForceLoginMethod, out.ForceLoginMethod)
		assert.Equal(t, in.ForceLoginGatewayURL, out.ForceLoginGatewayURL)
	})

	t.Run("empty omits the key", func(t *testing.T) {
		data, err := json.Marshal(Settings{ForceLoginMethod: "gateway"})
		require.NoError(t, err)

		var got map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotContains(t, got, "forceLoginGatewayUrl")
	})
}

func TestAPIKeySourceConstants(t *testing.T) {
	// The wire values are not derivable from the constant names — two carry
	// characters Go identifiers cannot ("/login managed key") or a casing
	// that differs from the identifier ("apiKeyHelper") — so pin them.
	assert.Equal(t, "ANTHROPIC_API_KEY", APIKeySourceAnthropicAPIKey)
	assert.Equal(t, "apiKeyHelper", APIKeySourceAPIKeyHelper)
	assert.Equal(t, "/login managed key", APIKeySourceLoginManagedKey)
	assert.Equal(t, "none", APIKeySourceNone)
}

func TestSystemMessageAPIKeySourceNone(t *testing.T) {
	// A subscription-authenticated session reports "none": no API key is in
	// use, which is not an error state. This is what CLI 2.1.222 emits under
	// a claude.ai OAuth login.
	msg, err := ParseMessage([]byte(`{
		"type": "system",
		"subtype": "init",
		"uuid": "550e8400-e29b-41d4-a716-446655440a00",
		"session_id": "sess_aks_001",
		"apiKeySource": "none",
		"cwd": "/workspace",
		"tools": [],
		"mcp_servers": [],
		"model": "claude-opus-4-5-20250929",
		"permissionMode": "default",
		"slash_commands": [],
		"output_style": "default"
	}`))
	require.NoError(t, err)

	systemMsg, ok := msg.(SystemMessage)
	require.True(t, ok)
	assert.Equal(t, APIKeySourceNone, systemMsg.APIKeySource)
}

func TestAssistantMessageErrorAccountOnHold(t *testing.T) {
	assert.Equal(t,
		AssistantMessageError("account_on_hold"),
		AssistantMessageErrorAccountOnHold)
}

func TestSettingsParityV0_3_241(t *testing.T) {
	enabled := true
	syncOff := false
	autoContinue := true

	settings := Settings{
		SyncClaudeAiSkills:       &syncOff,
		KeybindingFlavor:         KeybindingFlavorReadline,
		AutoContinueAtUsageLimit: &autoContinue,
		Worktree: &SettingsWorktree{
			BgIsolation: "worktree",
			Location:    "~/src/worktrees",
		},
		ModelSettings: map[string]SettingsModel{
			"claude-opus-4-7": {EffortLevel: EffortXHigh},
			"claude-sonnet-5": {EffortLevel: EffortLow},
		},
		Spellcheck: &SettingsSpellcheck{
			Enabled:  &enabled,
			Checker:  "hunspell",
			Language: "en_GB",
			Color:    "ansi256(203)",
		},
	}

	data, err := json.Marshal(settings)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, false, got["syncClaudeAiSkills"])
	assert.Equal(t, "readline", got["keybindingFlavor"])
	assert.Equal(t, true, got["autoContinueAtUsageLimit"])

	worktree, ok := got["worktree"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "~/src/worktrees", worktree["location"])

	modelSettings, ok := got["modelSettings"].(map[string]interface{})
	require.True(t, ok)
	opus, ok := modelSettings["claude-opus-4-7"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "xhigh", opus["effortLevel"])

	spellcheck, ok := got["spellcheck"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, spellcheck["enabled"])
	assert.Equal(t, "hunspell", spellcheck["checker"])
	assert.Equal(t, "en_GB", spellcheck["language"])
	assert.Equal(t, "ansi256(203)", spellcheck["color"])

	var back Settings
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, settings, back)
}

func TestSettingsParityV0_3_241OmitEmpty(t *testing.T) {
	data, err := json.Marshal(Settings{})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))

	for _, key := range []string{
		"syncClaudeAiSkills",
		"keybindingFlavor",
		"autoContinueAtUsageLimit",
		"modelSettings",
		"spellcheck",
	} {
		assert.NotContains(t, got, key)
	}
}

func TestSettingsSyncClaudeAiSkillsFalseSurvives(t *testing.T) {
	// Only false is honored upstream, so it is the one value that must not be
	// swallowed by omitempty. A *bool is what makes that work.
	off := false
	data, err := json.Marshal(Settings{SyncClaudeAiSkills: &off})
	require.NoError(t, err)
	assert.JSONEq(t, `{"syncClaudeAiSkills": false}`, string(data))
}
