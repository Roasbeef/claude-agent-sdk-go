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

func TestSettingsManagedOrgFieldsJSON(t *testing.T) {
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
