package claudeagent

import (
	"context"
	"encoding/json"
	"fmt"
)

// Options holds configuration for a Claude agent client.
//
// Options are provided via functional options passed to NewClient.
// All fields have sensible defaults and can be selectively overridden.
type Options struct {
	// SystemPrompt is the system prompt sent to Claude.
	// Can be a string or SystemPromptPreset for preset prompts.
	SystemPrompt string

	// SystemPromptPreset uses a preset system prompt configuration.
	// Use "claude_code" to get Claude Code's default system prompt.
	SystemPromptPreset *SystemPromptConfig

	// Model specifies which Claude model to use.
	// Default: "claude-sonnet-4-5-20250929"
	Model string

	// MainAgent names the agent to apply to the main thread.
	MainAgent string

	// FallbackModel is the fallback model list to use if the primary model is
	// overloaded or unavailable. Accepts a comma-separated list to try in order;
	// the primary model is re-tried at the start of each user turn.
	FallbackModel string

	// CLIPath is the path to the Claude Code CLI executable.
	// If empty, the CLI will be discovered from PATH.
	CLIPath string

	// ExtraArgs are arbitrary Claude CLI flags appended after SDK-managed flags.
	// A nil value emits a bare flag.
	ExtraArgs map[string]*string

	// Cwd is the current working directory for the agent.
	// Default: process.cwd() equivalent
	Cwd string

	// AdditionalDirectories are additional directories Claude can access.
	AdditionalDirectories []string

	// Env is a map of environment variables to overlay onto the CLI
	// subprocess environment. Entries are appended to os.Environ()
	// before spawn, so parent-process variables like PATH and HOME
	// remain visible to the subprocess by default.
	//
	// Note: this is the inverse of the TypeScript SDK's Options.env,
	// which REPLACES the subprocess environment entirely (sdk.d.ts
	// v0.3.150 L1326-L1332). Callers porting from TS that relied on
	// the replace-semantics must clear inherited variables themselves;
	// there is no opt-in flag for replace mode.
	//
	// ANTHROPIC_API_KEY should be set here or in the parent
	// environment.
	Env map[string]string

	// PermissionMode controls tool execution permissions.
	// Default: PermissionModeDefault
	PermissionMode PermissionMode

	// AllowDangerouslySkipPermissions enables bypassing permissions.
	// Required when using PermissionModeBypassAll.
	AllowDangerouslySkipPermissions bool

	// CanUseTool is a callback invoked before tool execution.
	// Return PermissionAllow to proceed or PermissionDeny to block.
	CanUseTool CanUseToolFunc

	// OnElicitation handles MCP server requests for user input.
	OnElicitation OnElicitationFunc

	// OnUserDialog handles request_user_dialog control requests from the CLI.
	// Each dialogKind defines its own payload + result shape. When unset or
	// when the callback answers cancelled, the CLI applies the dialog's
	// default behavior. Hosts MUST answer unrecognized dialogKind values
	// with cancelled.
	OnUserDialog OnUserDialogFunc

	// SupportedDialogKinds declares the request_user_dialog dialog_kind
	// values this consumer's OnUserDialog can actually render (e.g.
	// "refusal_fallback_prompt"). The CLI fails closed on absence: a dialog
	// kind not declared here is never emitted to this session, and the flow
	// behind it degrades to its no-dialog behavior. Requires OnUserDialog —
	// a non-empty list without the callback is rejected at initialize. On
	// multi-client sessions the first-attached client's declaration wins.
	SupportedDialogKinds []string

	// PerTaskStopAffordance declares that this consumer renders a per-task
	// stop control wired to the stop_task control request, so a user can stop
	// one background task without stopping the rest.
	//
	// It changes what an interrupt does. Declared, an interrupt on an
	// open-input (interactive stream-json) session aborts only the current
	// turn and spares running background agents and workflows; the user stops
	// those one at a time through the consumer's own affordance. The CLI
	// fails closed on absence — an interrupt kills background tasks, because
	// a spared runaway task would otherwise be unstoppable from a consumer
	// that cannot render a stop control.
	//
	// A closed-input run is the exception. The string-prompt form and -p both
	// close stdin, and with stdin closed a stop_task control could never be
	// delivered, so hold-back tasks are killed at the held-result release
	// regardless of this declaration.
	//
	// First-attached-client wins on multi-client sessions; later initializes
	// do not change it (sdk.d.ts v0.3.251 L1667).
	PerTaskStopAffordance *bool

	// GetHostAuthToken handles host-auth-token refresh requests from the CLI.
	// If unset, the SDK replies with an error response.
	GetHostAuthToken GetHostAuthTokenFunc

	// Hooks register lifecycle callbacks for events like tool use.
	Hooks map[HookType][]HookConfig

	// Agents defines specialized subagents for task delegation.
	Agents map[string]AgentDefinition

	// PlanModeInstructions customizes the plan-mode workflow body.
	PlanModeInstructions string

	// Title sets a custom session title.
	Title string

	// Skills limits main-session skills to the named allowlist.
	Skills []string

	// PromptSuggestions enables next-prompt suggestion events.
	PromptSuggestions *bool

	// AgentProgressSummaries enables agent progress summary events.
	AgentProgressSummaries *bool

	// ForwardSubagentText surfaces subagent text in the main stream.
	ForwardSubagentText *bool

	// ToolAliases maps a tool name to a redirect target. When the model
	// emits a `tool_use` whose name is a key in the map, the execution path
	// resolves the mapped value instead. Single-hop (cycles do not loop).
	// Complementary to DisallowedTools; alias only affects name-based
	// lookup of model-emitted tool_use blocks.
	ToolAliases map[string]string

	// SessionOptions configure session behavior (create/resume/fork).
	SessionOptions SessionOptions

	// MCPServers configure MCP servers for custom tool integration.
	MCPServers map[string]MCPServerConfig

	// SkillsConfig controls Skills loading behavior.
	SkillsConfig SkillsConfig

	// SettingSources controls which filesystem settings to load.
	// Options: "user", "project", "local"
	// When omitted, no filesystem settings are loaded (SDK default).
	SettingSources []SettingSource

	// SettingsPath loads explicit settings from the given JSON file path.
	// Mutually exclusive with Settings.
	SettingsPath string

	// Settings supplies inline Claude Code settings as JSON.
	// Mutually exclusive with SettingsPath.
	Settings *Settings

	// ManagedSettings supplies inline managed settings as JSON.
	ManagedSettings *Settings

	// Sandbox configures sandbox behavior programmatically.
	Sandbox *SandboxSettings

	// Betas enables beta features.
	// Each beta header is passed to the CLI via --betas as a comma-separated
	// list. Example: []string{"context-1m-2025-08-07"}.
	Betas []string

	// Debug enables debug logging from the CLI.
	Debug bool

	// DebugFile writes debug logs to the specified file.
	// When set, the CLI implicitly enables debug logging.
	DebugFile string

	// ExcludeDynamicSystemPromptSections moves per-machine sections (cwd,
	// env info, memory paths, git status) from the system prompt into the
	// first user message. This improves cross-invocation prompt-cache reuse
	// by keeping the system prompt prefix stable across runs.
	//
	// The CLI only honors this flag with the default system prompt — it is
	// ignored when SystemPrompt is set to a custom string.
	ExcludeDynamicSystemPromptSections bool

	// Plugins loads custom plugins from local paths.
	Plugins []PluginConfig

	// OutputFormat defines structured output format for agent results.
	OutputFormat *OutputFormat

	// AllowedTools is a list of allowed tool names.
	// If empty, all tools are allowed.
	//
	// Note: passing "Skill" here is deprecated as of TS SDK v0.3.150
	// (sdk.d.ts L1265-L1268). Per-agent skill preloading lives on
	// AgentDefinition.Skills; a top-level user-facing skills option
	// is tracked separately and is not yet exposed by the Go SDK
	// (the existing Options.Skills field mirrors the control-init
	// loading allowlist, which is a different surface).
	AllowedTools []string

	// DisallowedTools is a list of tool names to explicitly disallow for this
	// agent. MCP server-level specs (mcp__server, mcp__server__*, mcp__*)
	// remove every tool from the named server (or all MCP tools).
	DisallowedTools []string

	// Tools configures available built-in tools.
	// Can be a list of tool names or use preset "claude_code".
	//
	// Note: native builds may provide search via Bash `find`/`grep`
	// instead of the dedicated Grep/Glob tools. List Grep/Glob here
	// or in AllowedTools to get them.
	Tools *ToolsConfig

	// Thinking controls Claude's thinking/reasoning behavior.
	// When set, takes precedence over MaxThinkingTokens.
	Thinking *ThinkingConfig

	// Effort controls how much effort Claude puts into its response.
	Effort EffortLevel

	// MaxBudgetUsd is the maximum budget in USD for the query.
	MaxBudgetUsd *float64

	// TaskBudget is the maximum task budget for the query.
	TaskBudget *TaskBudget

	// MaxThinkingTokens is the maximum tokens for thinking process.
	//
	// Deprecated: Use Thinking instead.
	MaxThinkingTokens *int

	// MaxTurns is the maximum conversation turns.
	MaxTurns *int

	// EnableFileCheckpointing enables file change tracking for rewinding.
	EnableFileCheckpointing bool

	// IncludePartialMessages includes partial message events in stream.
	IncludePartialMessages bool

	// Continue continues the most recent conversation.
	Continue bool

	// Stderr is a callback for stderr output from the CLI.
	Stderr func(data string)

	// Transport, when non-nil, is used in place of the default subprocess
	// transport. Primarily for testing with mock transports; real users should
	// leave this unset.
	Transport Transport `json:"-"`

	// Verbose enables debug logging from the CLI.
	Verbose bool

	// NoSessionPersistence disables session persistence - sessions will not
	// be saved to disk and cannot be resumed. Useful for testing.
	NoSessionPersistence bool

	// ConfigDir overrides the Claude config directory.
	// By default, Claude uses ~/.claude (or ~/.config/claude).
	// Set this to isolate from user settings, hooks, and sessions.
	// The CLAUDE_CONFIG_DIR environment variable is set when this is specified.
	ConfigDir string

	// StrictMCPConfig, when true, only uses MCP servers from MCPServers
	// and explicitly passed agent definitions, ignoring all other MCP
	// configurations from settings files, plugins, and on-disk agent
	// frontmatter. Maps to the CLI --strict-mcp-config flag.
	StrictMCPConfig bool

	// SDKMcpServers are in-process MCP servers that run within the SDK.
	// Tool calls to these servers are routed through the control channel
	// rather than spawning separate processes.
	// Use WithMcpServer() to add servers.
	SDKMcpServers map[string]*McpServer

	// AskUserQuestionHandler handles questions from Claude synchronously.
	// When Claude invokes the AskUserQuestion tool, this handler is called
	// with the question set. Return answers or an error.
	// If nil, questions are routed to the Questions() iterator.
	AskUserQuestionHandler AskUserQuestionHandler

	// TaskStore is a custom task storage backend for the task list system.
	// If nil, the default FileTaskStore is used when TaskManager is accessed.
	TaskStore TaskStore

	// TaskListID is the shared task list identifier.
	// When set, CLAUDE_CODE_TASK_LIST_ID is passed to the CLI subprocess,
	// enabling multiple instances to share the same task list.
	// Tasks persist at ~/.claude/tasks/{TaskListID}/.
	TaskListID string
}

// SystemPromptConfig represents system prompt configuration.
type SystemPromptConfig struct {
	Type   string // "preset"
	Preset string // "claude_code"
	Append string // Additional instructions to append
}

// SettingSource represents a filesystem settings source.
type SettingSource string

const (
	// SettingSourceUser loads global user settings (~/.claude/settings.json).
	SettingSourceUser SettingSource = "user"
	// SettingSourceProject loads shared project settings (.claude/settings.json).
	SettingSourceProject SettingSource = "project"
	// SettingSourceLocal loads local project settings (.claude/settings.local.json).
	SettingSourceLocal SettingSource = "local"
)

// Settings configures Claude Code settings supplied via --settings or
// --managed-settings.
type Settings struct {
	Schema              string `json:"$schema,omitempty"`
	APIKeyHelper        string `json:"apiKeyHelper,omitempty"`
	ProxyAuthHelper     string `json:"proxyAuthHelper,omitempty"`
	AWSCredentialExport string `json:"awsCredentialExport,omitempty"`
	AWSAuthRefresh      string `json:"awsAuthRefresh,omitempty"`
	GCPAuthRefresh      string `json:"gcpAuthRefresh,omitempty"`
	// PolicyHelper configures the admin-controlled policy executable invoked at startup to compute managed settings. Honored only from policy sources. Mirrors sdk.d.ts v0.3.150 L3993.
	PolicyHelper      *SettingsPolicyHelper   `json:"policyHelper,omitempty"`
	FileSuggestion    *SettingsFileSuggestion `json:"fileSuggestion,omitempty"`
	RespectGitignore  *bool                   `json:"respectGitignore,omitempty"`
	CleanupPeriodDays *int                    `json:"cleanupPeriodDays,omitempty"`
	// SyncClaudeAiSkills turns off syncing of the skills enabled on
	// claude.ai. Only false is honored: the feature is enabled server-side per
	// account, so setting true does not turn it on early. Not read from
	// project settings (.claude/settings.json), and only applies when signed
	// in with a Claude account.
	//
	// The effect of a false depends on which file carries it. In user or
	// managed settings it is destructive-ish: nothing more is downloaded,
	// previously synced skills under ~/.claude/skills/synced stop running and
	// are hidden from sessions started afterwards, and at the next launch they
	// move to ~/.claude/skills/.trash — deleted after CleanupPeriodDays, and
	// re-downloaded rather than restored if the setting is reverted. In
	// .claude/settings.local.json or --settings it is scoped and reversible:
	// downloads stop and synced skills are blocked and hidden for that
	// workspace or invocation only, with nothing moved (sdk.d.ts v0.3.241
	// L5392).
	SyncClaudeAiSkills         *bool                            `json:"syncClaudeAiSkills,omitempty"`
	SkillListingMaxDescChars   *int                             `json:"skillListingMaxDescChars,omitempty"`
	SkillListingBudgetFraction *float64                         `json:"skillListingBudgetFraction,omitempty"`
	WSLInheritsWindowsSettings *bool                            `json:"wslInheritsWindowsSettings,omitempty"`
	Env                        map[string]string                `json:"env,omitempty"`
	Attribution                *SettingsAttribution             `json:"attribution,omitempty"`
	IncludeCoAuthoredBy        *bool                            `json:"includeCoAuthoredBy,omitempty"`
	IncludeGitInstructions     *bool                            `json:"includeGitInstructions,omitempty"`
	Permissions                *SettingsPermissions             `json:"permissions,omitempty"`
	Model                      string                           `json:"model,omitempty"`
	FallbackModel              []string                         `json:"fallbackModel,omitempty"`
	AvailableModels            []string                         `json:"availableModels,omitempty"`
	ModelOverrides             map[string]string                `json:"modelOverrides,omitempty"`
	EnableAllProjectMCPServers *bool                            `json:"enableAllProjectMcpServers,omitempty"`
	EnabledMCPJSONServers      []string                         `json:"enabledMcpjsonServers,omitempty"`
	DisabledMCPJSONServers     []string                         `json:"disabledMcpjsonServers,omitempty"`
	SkillOverrides             map[string]SettingsSkillOverride `json:"skillOverrides,omitempty"`
	AllowedMCPServers          []SettingsMCPServerMatcher       `json:"allowedMcpServers,omitempty"`
	DeniedMCPServers           []SettingsMCPServerMatcher       `json:"deniedMcpServers,omitempty"`
	Hooks                      map[string][]SettingsHookMatcher `json:"hooks,omitempty"`
	Worktree                   *SettingsWorktree                `json:"worktree,omitempty"`
	DisableAllHooks            *bool                            `json:"disableAllHooks,omitempty"`
	DisableSkillShellExecution *bool                            `json:"disableSkillShellExecution,omitempty"`
	DefaultShell               string                           `json:"defaultShell,omitempty"`
	// RespondToBashCommands controls whether Claude responds after an
	// input-box ! bash command runs. Set to false to add the command output to
	// context without a response. Default true. Mirrors sdk.d.ts v0.3.195 L5032.
	RespondToBashCommands           *bool                          `json:"respondToBashCommands,omitempty"`
	AllowManagedHooksOnly           *bool                          `json:"allowManagedHooksOnly,omitempty"`
	AllowedHTTPHookURLs             []string                       `json:"allowedHttpHookUrls,omitempty"`
	HTTPHookAllowedEnvVars          []string                       `json:"httpHookAllowedEnvVars,omitempty"`
	AllowManagedPermissionRulesOnly *bool                          `json:"allowManagedPermissionRulesOnly,omitempty"`
	AllowManagedMCPServersOnly      *bool                          `json:"allowManagedMcpServersOnly,omitempty"`
	StrictPluginOnlyCustomization   interface{}                    `json:"strictPluginOnlyCustomization,omitempty"`
	StatusLine                      *SettingsCommand               `json:"statusLine,omitempty"`
	PRURLTemplate                   string                         `json:"prUrlTemplate,omitempty"`
	SubagentStatusLine              *SettingsCommand               `json:"subagentStatusLine,omitempty"`
	EnabledPlugins                  map[string]interface{}         `json:"enabledPlugins,omitempty"`
	ExtraKnownMarketplaces          map[string]SettingsMarketplace `json:"extraKnownMarketplaces,omitempty"`
	// StrictKnownMarketplaces and BlockedMarketplaces are the managed-settings
	// policy lists, and are the only place a github entry may use the
	// owner-wildcard form {"source":"github","repo":"owner/*"} to match every
	// repository under that owner. Everywhere else — marketplace add,
	// ExtraKnownMarketplaces, known_marketplaces.json — "repo" must name a
	// single repository, and a wildcard is taken literally and fails to clone
	// (sdk.d.ts v0.3.226 L5909, L6112).
	StrictKnownMarketplaces []SettingsMarketplaceSource `json:"strictKnownMarketplaces,omitempty"`
	BlockedMarketplaces     []SettingsMarketplaceSource `json:"blockedMarketplaces,omitempty"`
	// AdditionalMarketplaces is read exactly as if it were spelled
	// ExtraKnownMarketplaces. Setting both in one file makes this one lose: it
	// is ignored with a warning. Claude Code may also rewrite the key back to
	// extraKnownMarketplaces when it updates the file.
	//
	// Prefer ExtraKnownMarketplaces while older clients still share the same
	// settings: a client predating the alias ignores it outright. Mirrors
	// sdk.d.ts v0.3.241 L6152, which dropped the earlier claim that such a
	// client's settings sync re-uploads the file as declaring no
	// marketplaces.
	AdditionalMarketplaces map[string]SettingsMarketplace `json:"additionalMarketplaces,omitempty"`
	// AllowedMarketplaces is read exactly as if it were spelled
	// StrictKnownMarketplaces, and like it is honored only from managed
	// settings. Setting both in one file makes this one lose: it is ignored
	// with a warning. Keep using StrictKnownMarketplaces when the allowlist
	// must also bind clients that predate the alias, since they ignore this
	// spelling and would run unrestricted. Mirrors sdk.d.ts v0.3.233 L6468.
	AllowedMarketplaces []SettingsMarketplaceSource `json:"allowedMarketplaces,omitempty"`
	// DisableSideloadFlags, when true and set in managed settings, rejects the
	// --plugin-dir, --plugin-url, --agents, and non-sdk --mcp-config CLI flags
	// at startup, closing the CLI-flag bypass of strictKnownMarketplaces.
	// Honored only from managed settings. Mirrors sdk.d.ts v0.3.195 L5712.
	DisableSideloadFlags *bool `json:"disableSideloadFlags,omitempty"`
	// DisableCommandPluginSources gates the "command" plugin source, whose
	// plugin directory is produced by running a marketplace-declared command on
	// this machine (see SettingsMarketplaceSourceCommand). True means
	// command-sourced plugins are never installed, updated, or re-resolved, so
	// the command never runs; false allows them explicitly. Left nil it follows
	// AllowManagedHooksOnly, on the reasoning that an org restricting hook
	// execution to managed settings wants arbitrary plugin-resolution commands
	// off too. Honored only from managed settings. Mirrors sdk.d.ts v0.3.233
	// L6898.
	DisableCommandPluginSources *bool `json:"disableCommandPluginSources,omitempty"`
	// PluginSuggestionMarketplaces names managed marketplaces whose plugins may surface as contextual install suggestions. Honored only from managed settings. Mirrors sdk.d.ts v0.3.168 L5242.
	PluginSuggestionMarketplaces []string `json:"pluginSuggestionMarketplaces,omitempty"`
	// ForceLoginMethod pins the login flow: "claudeai" for Claude Pro/Max, "console" for Console billing, "gateway" for the Cloud gateway OIDC device flow (the "gateway" variant was added in v0.3.168). Mirrors sdk.d.ts v0.3.168 L5246.
	ForceLoginMethod string `json:"forceLoginMethod,omitempty"`
	// ForceLoginGatewayURL is the Cloud gateway URL to pre-fill and
	// auto-connect to during login, and only means anything alongside
	// ForceLoginMethod "gateway". Honored only from admin-controlled managed
	// settings (MDM, managed-settings.json, policy helper) and ignored in
	// user, project, and remote-delivered settings, since pointing a login
	// flow at an attacker-chosen gateway is exactly what that tier exists to
	// prevent. Mirrors sdk.d.ts v0.3.233 L6914.
	ForceLoginGatewayURL       string                       `json:"forceLoginGatewayUrl,omitempty"`
	ForceLoginOrgUUID          interface{}                  `json:"forceLoginOrgUUID,omitempty"`
	ForceRemoteSettingsRefresh *bool                        `json:"forceRemoteSettingsRefresh,omitempty"`
	OtelHeadersHelper          string                       `json:"otelHeadersHelper,omitempty"`
	OutputStyle                string                       `json:"outputStyle,omitempty"`
	ViewMode                   string                       `json:"viewMode,omitempty"`
	Language                   string                       `json:"language,omitempty"`
	SkipWebFetchPreflight      *bool                        `json:"skipWebFetchPreflight,omitempty"`
	Sandbox                    *SettingsSandbox             `json:"sandbox,omitempty"`
	FeedbackSurveyRate         *float64                     `json:"feedbackSurveyRate,omitempty"`
	SpinnerTipsEnabled         *bool                        `json:"spinnerTipsEnabled,omitempty"`
	SpinnerVerbs               *SettingsSpinnerVerbs        `json:"spinnerVerbs,omitempty"`
	SpinnerTipsOverride        *SettingsSpinnerTipsOverride `json:"spinnerTipsOverride,omitempty"`
	SyntaxHighlightingDisabled *bool                        `json:"syntaxHighlightingDisabled,omitempty"`
	TerminalTitleFromRename    *bool                        `json:"terminalTitleFromRename,omitempty"`
	AlwaysThinkingEnabled      *bool                        `json:"alwaysThinkingEnabled,omitempty"`
	EffortLevel                EffortLevel                  `json:"effortLevel,omitempty"`
	// ModelSettings holds per-model settings keyed by canonical model name —
	// the per-model twin of EffortLevel above. Note the ceiling differs: a
	// persisted per-model effort tops out at "xhigh", where the init message's
	// applied effort also admits "max" (sdk.d.ts v0.3.241 L7415).
	ModelSettings map[string]SettingsModel `json:"modelSettings,omitempty"`
	// Spellcheck underlines misspelled words in the prompt input as you type,
	// using an installed aspell, hunspell or ispell. Read from user, flag and
	// managed settings only — the whole block from the highest-precedence of
	// those applies, and it is ignored in .claude/settings.json and
	// .claude/settings.local.json (sdk.d.ts v0.3.241 L7381).
	Spellcheck *SettingsSpellcheck `json:"spellcheck,omitempty"`
	// Ultracode enables session-scoped workflow orchestration, typically via --settings or apply_flag_settings. Mirrors sdk.d.ts v0.3.168 L5413.
	Ultracode                    *bool  `json:"ultracode,omitempty"`
	AutoCompactWindow            *int   `json:"autoCompactWindow,omitempty"`
	AdvisorModel                 string `json:"advisorModel,omitempty"`
	FastMode                     *bool  `json:"fastMode,omitempty"`
	FastModePerSessionOptIn      *bool  `json:"fastModePerSessionOptIn,omitempty"`
	PromptSuggestionEnabled      *bool  `json:"promptSuggestionEnabled,omitempty"`
	ShowClearContextOnPlanAccept *bool  `json:"showClearContextOnPlanAccept,omitempty"`
	// AskUserQuestionTimeout is the idle time before Claude's questions
	// auto-continue with any answers selected so far. Defaults to never —
	// auto-continue only runs when explicitly set to "60s", "5m", or "10m".
	// Mirrors sdk.d.ts v0.3.201.
	AskUserQuestionTimeout string `json:"askUserQuestionTimeout,omitempty"`
	// DialogExpiry caps how long a permission or user dialog forwarded to a
	// remote client stays parked awaiting an answer, and how long a HELD
	// cross-session message awaits approval, before either resolves to its safe
	// no-action default (cancelled / dropped-with-denial). "60s", "5m", "10m",
	// or "never" to disable the deadline; defaults to 5m, matching the
	// long-standing remote-dialog deadline. Local-only permission prompts are
	// unaffected. CLAUDE_CODE_USER_DIALOG_TIMEOUT_MS overrides it. Read from
	// trusted sources only, never a checked-in repo settings file (sdk.d.ts
	// v0.3.226 L6654).
	DialogExpiry string `json:"dialogExpiry,omitempty"`

	Agent                string                          `json:"agent,omitempty"`
	CompanyAnnouncements []string                        `json:"companyAnnouncements,omitempty"`
	PluginConfigs        map[string]SettingsPluginConfig `json:"pluginConfigs,omitempty"`
	Remote               *SettingsRemote                 `json:"remote,omitempty"`
	AutoUpdatesChannel   string                          `json:"autoUpdatesChannel,omitempty"`
	MinimumVersion       string                          `json:"minimumVersion,omitempty"`
	// RequiredMinimumVersion prevents startup below the managed minimum version. Honored only from managed (policy) settings. Mirrors sdk.d.ts v0.3.168 L5488.
	RequiredMinimumVersion string `json:"requiredMinimumVersion,omitempty"`
	// RequiredMaximumVersion prevents startup above the managed maximum version. Honored only from managed (policy) settings. Mirrors sdk.d.ts v0.3.168 L5492.
	RequiredMaximumVersion            string                  `json:"requiredMaximumVersion,omitempty"`
	PlansDirectory                    string                  `json:"plansDirectory,omitempty"`
	TUI                               string                  `json:"tui,omitempty"`
	Voice                             *SettingsVoice          `json:"voice,omitempty"`
	ChannelsEnabled                   *bool                   `json:"channelsEnabled,omitempty"`
	AllowedChannelPlugins             []SettingsChannelPlugin `json:"allowedChannelPlugins,omitempty"`
	PrefersReducedMotion              *bool                   `json:"prefersReducedMotion,omitempty"`
	AutoMemoryEnabled                 *bool                   `json:"autoMemoryEnabled,omitempty"`
	AutoMemoryDirectory               string                  `json:"autoMemoryDirectory,omitempty"`
	AutoDreamEnabled                  *bool                   `json:"autoDreamEnabled,omitempty"`
	ShowThinkingSummaries             *bool                   `json:"showThinkingSummaries,omitempty"`
	SkipDangerousModePermissionPrompt *bool                   `json:"skipDangerousModePermissionPrompt,omitempty"`
	DisableAutoMode                   string                  `json:"disableAutoMode,omitempty"`
	SSHConfigs                        []SettingsSSHConfig     `json:"sshConfigs,omitempty"`
	// ClaudeMD is CLAUDE.md-style instructions injected as organization-managed memory. Honored only from managed / policy settings. Mirrors sdk.d.ts v0.3.150 L5343.
	ClaudeMD           string   `json:"claudeMd,omitempty"`
	ClaudeMDExcludes   []string `json:"claudeMdExcludes,omitempty"`
	PluginTrustMessage string   `json:"pluginTrustMessage,omitempty"`
	Theme              string   `json:"theme,omitempty"`
	EditorMode         string   `json:"editorMode,omitempty"`
	// KeybindingFlavor selects which conventions the prompt's word-editing
	// keys follow. Empty string leaves the CLI default (KeybindingFlavorClassic)
	// in place (sdk.d.ts v0.3.241 L7640).
	KeybindingFlavor KeybindingFlavor `json:"keybindingFlavor,omitempty"`
	// VimInsertModeRemaps holds vim INSERT-mode key-sequence remaps, e.g.
	// {"jj": "<Esc>"}. Each key is exactly two printable characters typed in
	// sequence; "<Esc>" (return to NORMAL mode) is the only supported target.
	// Applies when EditorMode is "vim" (sdk.d.ts v0.3.215).
	VimInsertModeRemaps   map[string]interface{} `json:"vimInsertModeRemaps,omitempty"`
	Verbose               *bool                  `json:"verbose,omitempty"`
	PreferredNotifChannel string                 `json:"preferredNotifChannel,omitempty"`
	AutoCompactEnabled    *bool                  `json:"autoCompactEnabled,omitempty"`
	// PrecomputeCompactionEnabled precomputes the compaction summary in the
	// background before it is needed. Only applies when auto-compact is on.
	// Mirrors sdk.d.ts v0.3.220 L6545.
	PrecomputeCompactionEnabled *bool `json:"precomputeCompactionEnabled,omitempty"`
	// SwitchModelsOnFlag switches models automatically when safety measures flag a message. Mirrors sdk.d.ts v0.3.168 L5620.
	SwitchModelsOnFlag *bool `json:"switchModelsOnFlag,omitempty"`
	// AutoContinueAtUsageLimit waits for a claude.ai usage limit to reset and
	// continues the task automatically when one stops the session. When off,
	// the limit dialog offers the wait as a choice instead (sdk.d.ts v0.3.241
	// L7670).
	AutoContinueAtUsageLimit   *bool `json:"autoContinueAtUsageLimit,omitempty"`
	AutoScrollEnabled          *bool `json:"autoScrollEnabled,omitempty"`
	FileCheckpointingEnabled   *bool `json:"fileCheckpointingEnabled,omitempty"`
	ShowTurnDuration           *bool `json:"showTurnDuration,omitempty"`
	ShowMessageTimestamps      *bool `json:"showMessageTimestamps,omitempty"`
	TerminalProgressBarEnabled *bool `json:"terminalProgressBarEnabled,omitempty"`
	TodoFeatureEnabled         *bool `json:"todoFeatureEnabled,omitempty"`
	// WorkflowSizeGuideline is the advisory size guideline for the dynamic
	// workflows Claude writes: "small" (<5 agents), "medium" (<15, the
	// default), "large" (<50), or "unrestricted" (no guideline). A value here
	// takes precedence over the /config choice. Mirrors sdk.d.ts v0.3.220 L5396.
	WorkflowSizeGuideline string `json:"workflowSizeGuideline,omitempty"`
	// EmojiCompletionEnabled toggles the :emoji: shortcode typeahead (the
	// suggestion popup and :name: inline replacement). Enabled when absent or
	// true. Mirrors sdk.d.ts v0.3.220 L6342.
	EmojiCompletionEnabled *bool `json:"emojiCompletionEnabled,omitempty"`
	// VoiceEnabled enables voice mode (hold-to-talk dictation). Mirrors
	// sdk.d.ts v0.3.220 L6612.
	VoiceEnabled *bool `json:"voiceEnabled,omitempty"`
	// TeammateMode controls how spawned teammates execute: "auto", "tmux",
	// "iterm2", or "in-process" ("iterm2" added in sdk.d.ts v0.3.195 L6162).
	TeammateMode            string `json:"teammateMode,omitempty"`
	RemoteControlAtStartup  *bool  `json:"remoteControlAtStartup,omitempty"`
	AutoUploadSessions      *bool  `json:"autoUploadSessions,omitempty"`
	InputNeededNotifEnabled *bool  `json:"inputNeededNotifEnabled,omitempty"`
	AgentPushNotifEnabled   *bool  `json:"agentPushNotifEnabled,omitempty"`
	// Managed-org / policy tier additions (sdk.d.ts v0.3.150).
	// DisableAgentView disables the agent view. Typically set in managed settings. Mirrors sdk.d.ts v0.3.150 L4375.
	DisableAgentView *bool `json:"disableAgentView,omitempty"`
	// DisableRemoteControl disables Remote Control. Typically set in managed settings. Mirrors sdk.d.ts v0.3.150 L4379.
	DisableRemoteControl *bool `json:"disableRemoteControl,omitempty"`
	// DisableWorkflows disables the Workflows feature, also via CLAUDE_CODE_DISABLE_WORKFLOWS. Mirrors sdk.d.ts v0.3.168 L4573.
	DisableWorkflows *bool `json:"disableWorkflows,omitempty"`
	// EnableWorkflows enables or disables Workflows for this user. Unset leaves the plan default. Mirrors sdk.d.ts v0.3.168 L4577.
	EnableWorkflows *bool `json:"enableWorkflows,omitempty"`
	// WorkflowKeywordTriggerEnabled enables the "ultracode" keyword trigger for the Workflow tool. Mirrors sdk.d.ts v0.3.168 L4582.
	WorkflowKeywordTriggerEnabled *bool `json:"workflowKeywordTriggerEnabled,omitempty"`
	// AllowAllClaudeAiMcps lets claude.ai cloud MCP connectors load alongside managed-mcp.json. Mirrors sdk.d.ts v0.3.150 L4411.
	AllowAllClaudeAiMcps *bool `json:"allowAllClaudeAiMcps,omitempty"`
	// ParentSettingsBehavior controls whether the SDK parent tier layers under the admin tier. Mirrors sdk.d.ts v0.3.150 L5019.
	ParentSettingsBehavior string `json:"parentSettingsBehavior,omitempty"`
	// IsolatePeerMachines requires approval before SendMessage can reach peer sessions on other machines. Mirrors sdk.d.ts v0.3.150 L5407.
	IsolatePeerMachines *bool `json:"isolatePeerMachines,omitempty"`
	// DaemonColdStart controls daemon behavior when no background service is running. Mirrors sdk.d.ts v0.3.150 L5411.
	DaemonColdStart string `json:"daemonColdStart,omitempty"`
	// CrossSessionInbound governs inbound cross-session peer messages
	// (SendMessage from the user's other sessions): "accept" delivers them,
	// "hold" parks them for review without letting Claude act, "refuse" opts
	// this session out. An explicit value always wins. Unset means mode parity:
	// a message auto-delivers only when the sending session's permission-mode
	// class matches this one (bypass<->bypass or prompting<->prompting), a
	// mismatched sender is held for approval, and a sender that asserts no
	// class is held only while this session bypasses permission prompts
	// (sdk.d.ts v0.3.226 L6899).
	CrossSessionInbound string `json:"crossSessionInbound,omitempty"`
	// DisableDeepLinkRegistration prevents claude-cli:// protocol handler registration when set to "disable". Mirrors sdk.d.ts v0.3.150 L5427.
	DisableDeepLinkRegistration string `json:"disableDeepLinkRegistration,omitempty"`
	// DefaultView controls the default transcript view. Mirrors sdk.d.ts v0.3.150 L5431.
	DefaultView string `json:"defaultView,omitempty"`
	// EnforceAvailableModels restricts model selection to AvailableModels. Mirrors sdk.d.ts v0.3.177 L4608.
	EnforceAvailableModels *bool `json:"enforceAvailableModels,omitempty"`
	// DisableBundledSkills removes the skills and workflows that ship with Claude Code: bundled skills/workflows are removed entirely and built-in slash commands stay typable but hidden from the model. Plugins, .claude/skills/, and .claude/commands/ are unaffected. Equivalent to CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1. Mirrors sdk.d.ts v0.3.177 L4636.
	DisableBundledSkills *bool `json:"disableBundledSkills,omitempty"`
	// DisableArtifact disables the artifact view. Mirrors sdk.d.ts v0.3.177 L4905.
	DisableArtifact *bool `json:"disableArtifact,omitempty"`
	// EnableArtifact enables or disables the Artifact tool for this user.
	// Unset defaults to enabled once the feature is available. Mirrors
	// sdk.d.ts v0.3.201.
	EnableArtifact *bool `json:"enableArtifact,omitempty"`
	// FooterLinksRegexes adds clickable footer badges when a regex matches turn output. Read from user, flag, and managed settings only. At most 5 badges render. Mirrors sdk.d.ts v0.3.177 L4973.
	FooterLinksRegexes []SettingsFooterLinkRegex `json:"footerLinksRegexes,omitempty"`
	// WheelScrollAccelerationEnabled enables mouse-wheel scroll acceleration. Mirrors sdk.d.ts v0.3.177 L5988.
	WheelScrollAccelerationEnabled *bool `json:"wheelScrollAccelerationEnabled,omitempty"`
	// DisableClaudeAiConnectors, when true in any settings source, prevents claude.ai MCP cloud connectors from being auto-fetched or connected. Only gates auto-fetched connectors — a claudeai-proxy server passed explicitly (via --mcp-config or the SDK mcpServers option) still follows the normal MCP config trust flow. Any-source-true wins: a project can opt out, but a project-level false cannot override a user-level true. Mirrors sdk.d.ts v0.3.185 L4629.
	DisableClaudeAiConnectors *bool `json:"disableClaudeAiConnectors,omitempty"`
	// ProcessWrapper is the corporate launcher argv prefix for the
	// background-agent supervisor, the sessions and workers it hosts, and the
	// other covered background processes. Equivalent to the
	// CLAUDE_CODE_PROCESS_WRAPPER environment variable, which takes precedence
	// when set. Honored from managed settings, a --settings/SDK-supplied
	// settings file, and user settings, in that precedence order; project and
	// local settings are ignored (sdk.d.ts v0.3.215).
	ProcessWrapper string `json:"processWrapper,omitempty"`
	// FeedbackDrafts controls the model-drafted feedback tool (SendFeedback):
	// "notify" (default) shows a one-line notice when a draft is queued,
	// "quiet" shows only the footer counter, and "off" disables the tool
	// entirely so drafts are never queued (sdk.d.ts v0.3.215).
	FeedbackDrafts string `json:"feedbackDrafts,omitempty"`
}

// SettingsFooterLinkRegex is a footer-badge rule: when Pattern matches turn
// output, a badge linking to URL (with {name} placeholders filled from named
// regex capture groups) is rendered. Type is the config variant — this client
// understands "regex"; entries with other variants are preserved but skipped
// at runtime.
type SettingsFooterLinkRegex struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern,omitempty"`
	URL     string `json:"url,omitempty"`
	Label   string `json:"label,omitempty"`
}

type SettingsFileSuggestion struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// SettingsPolicyHelper configures the executable that computes managed settings at startup.
type SettingsPolicyHelper struct {
	Path              string `json:"path"`
	TimeoutMs         *int   `json:"timeoutMs,omitempty"`
	RefreshIntervalMs *int   `json:"refreshIntervalMs,omitempty"`
}

type SettingsAttribution struct {
	Commit *string `json:"commit,omitempty"`
	PR     *string `json:"pr,omitempty"`
	// SessionURL controls whether the claude.ai session link is appended to
	// commits and PRs created from web or Remote Control sessions (default:
	// true). Set false to omit the Claude-Session trailer and PR-body link.
	// Mirrors sdk.d.ts v0.3.185 L4551.
	SessionURL *bool `json:"sessionUrl,omitempty"`
}

type SettingsPermissions struct {
	Allow                        []string               `json:"allow,omitempty"`
	Deny                         []string               `json:"deny,omitempty"`
	Ask                          []string               `json:"ask,omitempty"`
	DefaultMode                  PermissionMode         `json:"defaultMode,omitempty"`
	DisableBypassPermissionsMode string                 `json:"disableBypassPermissionsMode,omitempty"`
	AdditionalDirectories        []string               `json:"additionalDirectories,omitempty"`
	Extra                        map[string]interface{} `json:"-"`
}

func (p SettingsPermissions) MarshalJSON() ([]byte, error) {
	type alias SettingsPermissions
	base, err := json.Marshal(alias(p))
	if err != nil {
		return nil, err
	}
	if len(p.Extra) == 0 {
		return base, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(base, &obj); err != nil {
		return nil, err
	}
	for k, v := range p.Extra {
		obj[k] = v
	}
	return json.Marshal(obj)
}

type SettingsSkillOverride string

const (
	SettingsSkillOverrideOn                SettingsSkillOverride = "on"
	SettingsSkillOverrideNameOnly          SettingsSkillOverride = "name-only"
	SettingsSkillOverrideUserInvocableOnly SettingsSkillOverride = "user-invocable-only"
	SettingsSkillOverrideOff               SettingsSkillOverride = "off"
)

type SettingsMCPServerMatcher struct {
	ServerName    string   `json:"serverName,omitempty"`
	ServerCommand []string `json:"serverCommand,omitempty"`
	ServerURL     string   `json:"serverUrl,omitempty"`
}

type SettingsHookMatcher struct {
	Matcher string         `json:"matcher,omitempty"`
	Hooks   []SettingsHook `json:"hooks"`
}

type SettingsHook struct {
	Type    string `json:"type"`
	Command string `json:"command,omitempty"`
	// Args is an argument list for hook exec form. When present, Command is
	// resolved as an executable and spawned directly with these arguments --
	// no shell. Path placeholders like ${CLAUDE_PLUGIN_ROOT} are substituted
	// per-element as plain strings, so paths with quotes, $, or backticks
	// never reach a shell parser. When absent, Command runs through a shell
	// (bash on POSIX, PowerShell on Windows without Git Bash).
	Args    []string               `json:"args,omitempty"`
	Prompt  string                 `json:"prompt,omitempty"`
	URL     string                 `json:"url,omitempty"`
	Server  string                 `json:"server,omitempty"`
	Tool    string                 `json:"tool,omitempty"`
	Input   map[string]interface{} `json:"input,omitempty"`
	If      string                 `json:"if,omitempty"`
	Shell   string                 `json:"shell,omitempty"`
	Timeout *int                   `json:"timeout,omitempty"`
	Model   string                 `json:"model,omitempty"`
	// ContinueOnBlock sets the continue value for the decision:"block" produced
	// when the hook returns ok=false. Default false (turn ends). Whether
	// continue=true lets the turn proceed depends on the event's decision:"block"
	// semantics: on PostToolUse, the reason is fed back to Claude and the turn
	// continues.
	ContinueOnBlock *bool             `json:"continueOnBlock,omitempty"`
	StatusMessage   string            `json:"statusMessage,omitempty"`
	Once            *bool             `json:"once,omitempty"`
	Async           *bool             `json:"async,omitempty"`
	AsyncRewake     *bool             `json:"asyncRewake,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	AllowedEnvVars  []string          `json:"allowedEnvVars,omitempty"`
}

func (h SettingsHook) MarshalJSON() ([]byte, error) {
	type alias SettingsHook

	data, err := json.Marshal(alias(h))
	if err != nil {
		return nil, err
	}
	if h.Args == nil {
		return data, nil
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	fields["args"] = h.Args

	return json.Marshal(fields)
}

type SettingsWorktree struct {
	SymlinkDirectories []string `json:"symlinkDirectories,omitempty"`
	SparsePaths        []string `json:"sparsePaths,omitempty"`
	// BaseRef selects which ref new worktrees branch from. Mirrors sdk.d.ts v0.3.150 L4364.
	BaseRef string `json:"baseRef,omitempty"`
	// BgIsolation selects the isolation mode for background sessions. Mirrors sdk.d.ts v0.3.150 L4368.
	BgIsolation string `json:"bgIsolation,omitempty"`
	// Location is the directory under which Claude Code Desktop creates the
	// worktrees of SSH sessions running on this machine, instead of
	// <project>/.claude/worktrees. An absolute path, or one starting with ~/.
	// Read by the desktop app from the SSH host user settings; a location
	// chosen in the desktop app's SSH connection settings takes precedence.
	//
	// The CLI does not read it — not for --worktree, not for EnterWorktree,
	// not for agent isolation — so setting it has no effect on an SDK-driven
	// session (sdk.d.ts v0.3.241 L5766).
	Location string `json:"location,omitempty"`
}

// KeybindingFlavor selects which conventions the prompt's word-editing keys
// follow.
type KeybindingFlavor string

const (
	// KeybindingFlavorClassic is the CLI default: Ctrl+W deletes the previous
	// word, and the word keys use Unicode word segmentation, so "foo_bar" and
	// "3.14" each count as one word.
	KeybindingFlavorClassic KeybindingFlavor = "classic"
	// KeybindingFlavorReadline matches Bash and other readline programs:
	// Ctrl+W deletes back to the previous whitespace; Alt+F and Alt+D stop at
	// the end of the current word and Ctrl+Y pastes back what Alt+D deleted.
	// For Alt+B, Alt+F, Alt+D, Ctrl/Option+Arrow and Option/Ctrl+Backspace a
	// word is a run of letters and digits, so punctuation separates words.
	KeybindingFlavorReadline KeybindingFlavor = "readline"
)

// SettingsModel holds the persisted settings for one model, keyed in
// Settings.ModelSettings by canonical model name.
//
// TS declares an open index signature on this entry, so the CLI may add keys
// here that this struct drops on a round trip.
type SettingsModel struct {
	// EffortLevel is the persisted effort level for this model. Unlike the
	// init message's applied effort, "max" is not a member here.
	EffortLevel EffortLevel `json:"effortLevel,omitempty"`
}

// SettingsSpellcheck configures prompt-input spell checking. It does nothing
// unless Enabled is true and one of aspell, hunspell or ispell is installed.
//
// TS declares an open index signature on this block, so the CLI may add keys
// here that this struct drops on a round trip.
type SettingsSpellcheck struct {
	// Enabled turns on spell checking of the prompt input. Default false.
	Enabled *bool `json:"enabled,omitempty"`
	// Checker selects which spell checker to run: "aspell", "hunspell",
	// "ispell", or "auto" (the default) for the first of those found on PATH.
	Checker string `json:"checker,omitempty"`
	// Language is the dictionary, passed to the checker as-is (aspell --lang,
	// hunspell -d, ispell -d), e.g. "en_GB". Names are checker-specific and
	// accept only letters, digits and _ - . , characters. Empty string leaves
	// the checker's own default.
	Language string `json:"language,omitempty"`
	// Color is the color of misspelled words, which are also underlined: a
	// terminal color name such as "red" or "magenta", "#rrggbb",
	// "rgb(r,g,b)", "ansi256(n)" or "ansi:<name>". Empty string leaves the
	// theme's error color.
	Color string `json:"color,omitempty"`
}

type SettingsCommand struct {
	Type            string `json:"type"`
	Command         string `json:"command"`
	Padding         *int   `json:"padding,omitempty"`
	RefreshInterval *int   `json:"refreshInterval,omitempty"`
	// HideVimModeIndicator hides the built-in -- INSERT -- / -- VISUAL -- indicator below the prompt. Mirrors sdk.d.ts v0.3.150 L4432.
	HideVimModeIndicator *bool `json:"hideVimModeIndicator,omitempty"`
}

type SettingsMarketplace struct {
	Source          SettingsMarketplaceSource `json:"source"`
	InstallLocation string                    `json:"installLocation,omitempty"`
	AutoUpdate      *bool                     `json:"autoUpdate,omitempty"`
}

// SettingsMarketplaceSource is the opaque marketplace source descriptor. Required key "source"
// selects the variant; remaining keys depend on the variant (e.g. "repo", "url", "package",
// "path", "ref", "sparsePaths", "skipLfs"). Per sdk.d.ts v0.3.168 L4695/L4717 (github) and
// L4895/L4917 (git), the optional "skipLfs": boolean key sets GIT_LFS_SKIP_SMUDGE=1 on clone/update.
//
// Sources that carry an inline "catalog" array accept two per-entry keys
// scoped to downloading that entry's "archive" source: "headers"
// (map[string]string) and "headersHelper" (string, a command printing a JSON
// object of headers). The helper here runs only when a user explicitly
// installs or updates that plugin — unlike the url source's helper, which is
// re-run on every marketplace refresh. Use an absolute path.
//
// Two asymmetries worth knowing before writing one. An entry declared in a
// settings file does not need "strict": false the way a manifest catalog entry
// does, because a settings file has no manifest fields to inline. And a
// declaration in *project* settings is not operator-authored, so
// request-routing and client-identity header names stay filtered there even
// though the same declaration in user or managed settings would pass.
// Mirrors sdk.d.ts v0.3.241 L6112.
type SettingsMarketplaceSource map[string]interface{}

// SettingsMarketplaceSourceKind is the discriminator value stored in a SettingsMarketplaceSource "source" entry.
type SettingsMarketplaceSourceKind string

const (
	// SettingsMarketplaceSourceGithub identifies a GitHub marketplace source. Mirrors sdk.d.ts v0.3.150 L4459.
	// Honors optional "skipLfs": boolean per sdk.d.ts v0.3.168 L4695.
	SettingsMarketplaceSourceGithub SettingsMarketplaceSourceKind = "github"
	// SettingsMarketplaceSourceGit identifies a Git marketplace source. Mirrors sdk.d.ts v0.3.150 L4466.
	// Honors optional "skipLfs": boolean per sdk.d.ts v0.3.168 L4717.
	SettingsMarketplaceSourceGit SettingsMarketplaceSourceKind = "git"
	// SettingsMarketplaceSourceNPM identifies an npm marketplace source. Mirrors sdk.d.ts v0.3.150 L4484.
	SettingsMarketplaceSourceNPM SettingsMarketplaceSourceKind = "npm"
	// SettingsMarketplaceSourceFile identifies a file marketplace source. Mirrors sdk.d.ts v0.3.150 L4500.
	SettingsMarketplaceSourceFile SettingsMarketplaceSourceKind = "file"
	// SettingsMarketplaceSourceDirectory identifies a directory marketplace source. Mirrors sdk.d.ts v0.3.150 L4515.
	SettingsMarketplaceSourceDirectory SettingsMarketplaceSourceKind = "directory"
	// SettingsMarketplaceSourceSkillsDir identifies a bare-tag skills-dir marketplace source. Mirrors sdk.d.ts v0.3.150 L4528.
	SettingsMarketplaceSourceSkillsDir SettingsMarketplaceSourceKind = "skills-dir"
	// SettingsMarketplaceSourceHostPattern identifies a hostPattern marketplace source. Mirrors sdk.d.ts v0.3.150 L4537.
	SettingsMarketplaceSourceHostPattern SettingsMarketplaceSourceKind = "hostPattern"
	// SettingsMarketplaceSourcePathPattern identifies a pathPattern marketplace source. Mirrors sdk.d.ts v0.3.150 L4555.
	SettingsMarketplaceSourcePathPattern SettingsMarketplaceSourceKind = "pathPattern"
	// SettingsMarketplaceSourceURL identifies a marketplace fetched from a
	// direct URL to a marketplace.json file. Honors "url": string, optional
	// "headers": map[string]string (custom HTTP headers, e.g. for auth), and
	// optional "headersHelper": string.
	//
	// The helper is a command that prints a JSON object of HTTP headers — a
	// short-lived auth token, typically. Its output overrides "headers" and,
	// like "headers", is inherited by same-origin archive downloads from this
	// marketplace. It runs from a fixed directory (the Claude config home,
	// never the session's), so a relative path will not resolve the way a
	// caller expects: give a bare command found via PATH, or an absolute
	// path. Re-run on later refreshes of this marketplace, so it must stay
	// callable for the life of the config, not just at install time.
	// Mirrors sdk.d.ts v0.3.241 L5930.
	SettingsMarketplaceSourceURL SettingsMarketplaceSourceKind = "url"
	// SettingsMarketplaceSourceArchive identifies a zip-archive plugin source.
	// Honors "url": string (HTTPS URL of the archive; the plugin root may sit at
	// the top or one directory deep, as a single wrapping directory is stripped)
	// and optional "sha256": string. When the digest is set every download is
	// verified against it and a mismatch refuses the install; it also serves as
	// the version identity when neither plugin.json nor the marketplace entry
	// declares a version. Note the update signal is the version string, so
	// changing only the digest while a version is declared does not trigger an
	// update. Mirrors sdk.d.ts v0.3.226 L5869.
	SettingsMarketplaceSourceArchive SettingsMarketplaceSourceKind = "archive"
	// SettingsMarketplaceSourceCommand identifies a plugin source whose
	// directory is produced by running a command on this machine. Honors
	// "command": string (a shell command that prints the absolute path of the
	// plugin directory on stdout, exactly one line, and exits 0, having left a
	// complete plugin there), optional "timeout": number (seconds to wait,
	// default 60) and optional "mode": "copy" | "link".
	//
	// The command runs through the platform shell from the user's home
	// directory, and is re-resolved on every install and update plus once per
	// session in the background, so the printed path may differ between runs.
	// Under the default "copy" mode the directory is copied into the plugin
	// cache and content-hashed, so it may be deleted afterwards. Under "link"
	// the cache entry points at the printed directory in place (no copy, no
	// size limit, macOS/Linux only), which means the directory has to stay
	// valid for as long as Claude Code runs, and a changed printed path is the
	// only signal of new content. See Settings.DisableCommandPluginSources for
	// the managed-settings gate. Mirrors sdk.d.ts v0.3.233 L5974.
	SettingsMarketplaceSourceCommand SettingsMarketplaceSourceKind = "command"
	// SettingsMarketplaceSourceUnsupported identifies a bare-tag unsupported marketplace source. Mirrors sdk.d.ts v0.3.150 L4619.
	// Honors optional "error": string carrying why the source was rejected, per sdk.d.ts v0.3.226 L5871.
	SettingsMarketplaceSourceUnsupported SettingsMarketplaceSourceKind = "unsupported"
)

type SettingsPluginConfig struct {
	MCPServers map[string]map[string]interface{} `json:"mcpServers,omitempty"`
	Options    map[string]interface{}            `json:"options,omitempty"`
}

type SettingsRemote struct {
	DefaultEnvironmentID string `json:"defaultEnvironmentId,omitempty"`
}

type SettingsVoice struct {
	Enabled    *bool  `json:"enabled,omitempty"`
	Mode       string `json:"mode,omitempty"`
	AutoSubmit *bool  `json:"autoSubmit,omitempty"`
}

type SettingsChannelPlugin struct {
	Marketplace string `json:"marketplace"`
	Plugin      string `json:"plugin"`
}

type SettingsSSHConfig struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	SSHHost         string `json:"sshHost"`
	SSHPort         *int   `json:"sshPort,omitempty"`
	SSHIdentityFile string `json:"sshIdentityFile,omitempty"`
	StartDirectory  string `json:"startDirectory,omitempty"`
}

type SettingsSpinnerVerbs struct {
	Mode  string   `json:"mode"`
	Verbs []string `json:"verbs"`
}

type SettingsSpinnerTipsOverride struct {
	ExcludeDefault *bool    `json:"excludeDefault,omitempty"`
	Tips           []string `json:"tips"`
}

type SettingsSandbox struct {
	Enabled                      *bool                      `json:"enabled,omitempty"`
	FailIfUnavailable            *bool                      `json:"failIfUnavailable,omitempty"`
	AutoAllowBashIfSandboxed     *bool                      `json:"autoAllowBashIfSandboxed,omitempty"`
	AllowUnsandboxedCommands     *bool                      `json:"allowUnsandboxedCommands,omitempty"`
	Network                      *SettingsSandboxNetwork    `json:"network,omitempty"`
	Filesystem                   *SettingsSandboxFilesystem `json:"filesystem,omitempty"`
	IgnoreViolations             map[string][]string        `json:"ignoreViolations,omitempty"`
	EnableWeakerNestedSandbox    *bool                      `json:"enableWeakerNestedSandbox,omitempty"`
	EnableWeakerNetworkIsolation *bool                      `json:"enableWeakerNetworkIsolation,omitempty"`
	ExcludedCommands             []string                   `json:"excludedCommands,omitempty"`
	Ripgrep                      *SettingsSandboxRipgrep    `json:"ripgrep,omitempty"`
	// BwrapPath is the absolute path to the bwrap (bubblewrap) binary on
	// Linux/WSL. Overrides auto-detection via PATH. Honored only from
	// admin-controlled managed settings. Mirrors sdk.d.ts v0.3.150 L5133.
	BwrapPath string `json:"bwrapPath,omitempty"`
	// SocatPath is the absolute path to the socat binary used by the sandbox
	// network proxy on Linux/WSL. Overrides auto-detection via PATH. Honored
	// only from admin-controlled managed settings. Mirrors sdk.d.ts v0.3.150 L5137.
	SocatPath string `json:"socatPath,omitempty"`
	// AllowAppleEvents (macOS only) lets sandboxed commands send Apple Events
	// (and look up the appleeventsd Mach service), needed for `open`,
	// `osascript`, and browser-based auth flows that open URLs. It REMOVES
	// code-execution isolation: sandboxed commands can launch other
	// applications unsandboxed with no user prompt and script running apps
	// (e.g. Terminal) subject to per-app TCC automation consent. Honored only
	// from user, managed/policy, or CLI (--settings) settings — project
	// settings are ignored. Default false. Mirrors sdk.d.ts v0.3.185 L5718.
	AllowAppleEvents *bool `json:"allowAppleEvents,omitempty"`
	// Credentials protects credential files/directories and environment
	// variables from sandboxed commands. Mirrors sdk.d.ts v0.3.195 L5822.
	Credentials *SettingsSandboxCredentials `json:"credentials,omitempty"`
	Extra       map[string]interface{}      `json:"-"`
}

// SettingsSandboxCredentials configures credential protection inside the
// sandbox.
type SettingsSandboxCredentials struct {
	// Files are credential files or directories to protect. "deny" blocks
	// reads inside the sandbox; "mask" substitutes a sentinel inside the
	// sandbox (whole-file, or per-Extract capture) and injects the real value
	// at the proxy. On macOS and Windows "mask" degrades to "deny".
	Files []SettingsSandboxCredentialFile `json:"files,omitempty"`
	// EnvVars are environment variables to protect. "deny" unsets the
	// variable for sandboxed commands; "mask" substitutes a sentinel inside
	// the sandbox and injects the real value at the proxy.
	EnvVars []SettingsSandboxCredentialEnvVar `json:"envVars,omitempty"`
	// AllowPlaintextInject allows sentinel->real substitution on the
	// plain-HTTP proxy path. Defaults to false: without TLS termination the
	// upstream identity is unverified and the credential travels in
	// cleartext. Set only for trusted-network test fixtures. Only honored
	// from user, managed/policy, or CLI (--settings) settings — project
	// settings are ignored (sdk.d.ts v0.3.201).
	AllowPlaintextInject *bool `json:"allowPlaintextInject,omitempty"`
	// AWSPairs groups masked env vars into AWS credential pairs for SigV4
	// re-signing when the variable names are non-standard; the conventional
	// AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN trio pairs
	// automatically when masked. Only honored from user, managed/policy, or
	// CLI (--settings) settings — project settings are ignored (sdk.d.ts
	// v0.3.226 L6510).
	AWSPairs []SettingsSandboxCredentialAWSPair `json:"awsPairs,omitempty"`
	// SigV4 sets the policy for AWS SigV4 request shapes the proxy cannot
	// re-sign when they reference a masked credential pair. Only honored from
	// user, managed/policy, or CLI (--settings) settings — project settings are
	// ignored (sdk.d.ts v0.3.226 L6527).
	SigV4 *SettingsSandboxCredentialSigV4 `json:"sigv4,omitempty"`
}

// SettingsSandboxCredentialAWSPair names the masked env vars that make up one
// AWS credential pair, so the proxy can re-sign SigV4 requests that were signed
// inside the sandbox with sentinel values.
//
// A member is only usable when its variable is forwarded as a whole-value
// "mask" entry: an entry carrying Extract or Decode does not qualify, since
// re-signing needs the whole real value. A pair whose key id or secret member
// is unusable never re-signs — it is dropped, unless it names a conventional
// AWS variable, in which case it is forwarded as an inert suppressor so
// implicit auto-pairing stays overridden. A pair whose only unusable member is
// the session token still re-signs, without an x-amz-security-token.
type SettingsSandboxCredentialAWSPair struct {
	// AccessKeyIDVar is the masked env var holding the AWS access key id.
	AccessKeyIDVar string `json:"accessKeyIdVar"`
	// SecretAccessKeyVar is the masked env var holding the AWS secret access
	// key.
	SecretAccessKeyVar string `json:"secretAccessKeyVar"`
	// SessionTokenVar is the masked env var holding the AWS session token, for
	// temporary credentials. When set, the proxy sends the real token as
	// x-amz-security-token on re-signed requests and adds it to the signed
	// header set if the client did not.
	SessionTokenVar string `json:"sessionTokenVar,omitempty"`
}

// SettingsSandboxCredentialSigV4 holds the per-shape policies for SigV4
// requests the proxy cannot re-sign. Each is "deny" (the default, fail closed)
// or "passthrough" (forward unre-signed, which the upstream will reject).
type SettingsSandboxCredentialSigV4 struct {
	// Streaming covers aws-chunked uploads (x-amz-content-sha256:
	// STREAMING-*): per-chunk signatures chain off the seed signature, so
	// re-signing would mean rewriting the body. "deny" fails closed with a 403.
	Streaming string `json:"streaming,omitempty"`
	// Presigned covers presigned URLs (X-Amz-Algorithm/X-Amz-Signature in the
	// query, no Authorization header) — the signature lives in the URL itself.
	Presigned string `json:"presigned,omitempty"`
	// SigV4A covers SigV4A (AWS4-ECDSA-P256-SHA256) asymmetric signatures:
	// there is no shared-key HMAC to recompute.
	SigV4A string `json:"sigv4a,omitempty"`
}

// SettingsSandboxCredentialFile protects a single credential file or directory.
type SettingsSandboxCredentialFile struct {
	// Path is the credential file or directory. Same resolution as
	// sandbox.filesystem.* paths: absolute, ~ expanded, or relative to the
	// settings file root.
	Path string `json:"path"`
	// Mode is the access mode for this path. "deny" blocks reads inside the
	// sandbox; "mask" shows sandboxed commands a sentinel-substituted copy
	// (whole-file, or only the spans captured by Extract) and the host proxy
	// swaps sentinel->real on egress to InjectHosts. On macOS and Windows
	// "mask" currently degrades to "deny" (sdk.d.ts v0.3.226 L6444).
	Mode string `json:"mode"`
	// Extract is an optional regex for structured masking. Applied globally to
	// the file; capture group 1 of each match is a credential value and only
	// those spans become sentinels, so a tool that parses the file (.netrc,
	// JSON, YAML) still succeeds. Without it the whole file content becomes one
	// sentinel. Accepted but ignored for "deny".
	Extract string `json:"extract,omitempty"`
	// OnExtractNoMatch governs the case where Extract matches nothing — or,
	// with Decode, where no candidate verifies. "warn" (default) leaves the
	// file readable as-is (fail-open, for credentials that may legitimately be
	// absent); "deny" degrades the entry to mode "deny" (fail-closed), and is
	// treated as "error" under sandbox.filesystem.disabled since read-denies
	// are dropped there; "error" aborts at sandbox setup.
	OnExtractNoMatch string `json:"onExtractNoMatch,omitempty"`
	// Decode names an encoded-credential format for "mask" mode. "jwt" locates
	// candidates with a built-in JWT regex (or Extract, if set), verifies they
	// really are JWTs, and swaps in a structurally valid fake so client-side
	// token parsing inside the sandbox keeps working.
	Decode string `json:"decode,omitempty"`
	// MaskClaims names top-level payload claims to mask inside each decoded
	// value instead of replacing the whole token; the token is rebuilt around
	// the modified payload so non-secret claims keep reading. Requires Decode.
	MaskClaims []string `json:"maskClaims,omitempty"`
	// MaskDuplicates also replaces verbatim occurrences of each captured value
	// outside the matched spans — for a secret repeated where the regex does
	// not reach. Matches raw substrings, so short or common values can corrupt
	// unrelated content; meant for long, high-entropy secrets. Defaults false.
	MaskDuplicates *bool `json:"maskDuplicates,omitempty"`
	// InjectHosts optionally narrows where the proxy substitutes this
	// credential. Only meaningful for "mask". If unset, defaults to
	// network.allowedDomains — injected at every reachable host. Each entry
	// must be reachable via network.allowedDomains.
	InjectHosts []string `json:"injectHosts,omitempty"`
}

// SettingsSandboxCredentialEnvVar protects a single environment variable.
type SettingsSandboxCredentialEnvVar struct {
	// Name is the environment variable name.
	Name string `json:"name"`
	// Mode is the access mode for this variable. "deny" unsets the variable
	// for sandboxed commands; "mask" shows sandboxed commands a sentinel
	// value and the host proxy swaps sentinel->real on egress to
	// InjectHosts (sdk.d.ts v0.3.201).
	Mode string `json:"mode"`
	// Extract is an optional regex for structured masking. Applied globally to
	// the value; capture group 1 of each match is a credential value and only
	// those spans become sentinels, so a composite value (a DATABASE_URL
	// connection string, a KEY:SECRET pair) still parses inside the sandbox.
	// Without it the whole value becomes one sentinel. Cannot be combined with
	// Decode — the decode path never consults it.
	Extract string `json:"extract,omitempty"`
	// OnExtractNoMatch governs the case where Extract matches nothing. "warn"
	// (default) lets the variable through unmasked (fail-open); "deny" unsets
	// it inside the sandbox (fail-closed); "error" aborts at sandbox setup.
	// Only meaningful when Mode is "mask" and Extract is set without Decode: a
	// mask entry carrying Decode takes the decode path and never consults this
	// field, so "deny" and "error" are rejected there and only "warn" is
	// accepted.
	OnExtractNoMatch string `json:"onExtractNoMatch,omitempty"`
	// Decode names an encoded-credential format for "mask" mode. "jwt"
	// verifies the whole value really is a JWT and swaps in a structurally
	// valid fake so client-side token parsing inside the sandbox keeps
	// working; a value that does not verify is left unmasked with a stderr
	// warning. Cannot be combined with Extract.
	Decode string `json:"decode,omitempty"`
	// MaskClaims names top-level payload claims to mask inside the decoded
	// value instead of replacing the whole token; the token is rebuilt around
	// the modified payload so claim-reading clients keep working. Requires
	// Decode.
	MaskClaims []string `json:"maskClaims,omitempty"`
	// InjectHosts optionally narrows where the proxy substitutes this
	// credential. Only meaningful when Mode is "mask"; accepted but ignored
	// for "deny". If unset, defaults to network.allowedDomains — the
	// credential is injected at every reachable host. Each entry must be
	// reachable via network.allowedDomains.
	InjectHosts []string `json:"injectHosts,omitempty"`
}

func (s SettingsSandbox) MarshalJSON() ([]byte, error) {
	type alias SettingsSandbox
	base, err := json.Marshal(alias(s))
	if err != nil {
		return nil, err
	}
	if len(s.Extra) == 0 {
		return base, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(base, &obj); err != nil {
		return nil, err
	}
	for k, v := range s.Extra {
		obj[k] = v
	}
	return json.Marshal(obj)
}

type SettingsSandboxNetwork struct {
	AllowedDomains          []string `json:"allowedDomains,omitempty"`
	DeniedDomains           []string `json:"deniedDomains,omitempty"`
	AllowManagedDomainsOnly *bool    `json:"allowManagedDomainsOnly,omitempty"`
	AllowUnixSockets        []string `json:"allowUnixSockets,omitempty"`
	AllowAllUnixSockets     *bool    `json:"allowAllUnixSockets,omitempty"`
	AllowLocalBinding       *bool    `json:"allowLocalBinding,omitempty"`
	AllowMachLookup         []string `json:"allowMachLookup,omitempty"`
	HTTPProxyPort           *int     `json:"httpProxyPort,omitempty"`
	SocksProxyPort          *int     `json:"socksProxyPort,omitempty"`
	// StrictAllowlist, when true, makes the sandbox runtime deterministically
	// deny hosts not in AllowedDomains instead of prompting. Enforced for
	// sandboxed commands only — in-process tools such as WebFetch are not gated.
	// Honored only from user, managed/policy, or CLI (--settings) settings;
	// project settings are ignored. Mirrors sdk.d.ts v0.3.220 L6154.
	StrictAllowlist *bool `json:"strictAllowlist,omitempty"`
	// TLSTerminate enables in-process TLS termination so the per-request
	// filter can inspect HTTPS request bodies. Provide a CA cert+key, or
	// leave both empty so sandbox-runtime generates an ephemeral pair for
	// the session. Experimental. Mirrors sdk.d.ts v0.3.150 L5086.
	TLSTerminate *SettingsSandboxTLSTerminate `json:"tlsTerminate,omitempty"`
}

type SettingsSandboxFilesystem struct {
	AllowWrite                []string `json:"allowWrite,omitempty"`
	DenyWrite                 []string `json:"denyWrite,omitempty"`
	DenyRead                  []string `json:"denyRead,omitempty"`
	AllowRead                 []string `json:"allowRead,omitempty"`
	AllowManagedReadPathsOnly *bool    `json:"allowManagedReadPathsOnly,omitempty"`
	// Disabled (macOS and Linux/WSL only), when true, skips filesystem
	// isolation entirely while keeping network and seccomp isolation — sandboxed
	// commands get unrestricted host filesystem access; egress stays confined to
	// network.allowedDomains. Ignored on native Windows. Intended for egress-
	// control deployments. Honored only from user, managed/policy, or CLI
	// (--settings) settings; project settings are ignored. Mirrors sdk.d.ts
	// v0.3.220 L6205.
	Disabled *bool `json:"disabled,omitempty"`
}

type SettingsSandboxRipgrep struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// SettingsSandboxTLSTerminate enables in-process TLS termination so the
// per-request filter can see HTTPS request bodies. Provide a CA cert+key, or
// omit both to have sandbox-runtime generate an ephemeral one for the session.
// Experimental. Mirrors sdk.d.ts v0.3.150 L5086-L5092.
type SettingsSandboxTLSTerminate struct {
	CACertPath string `json:"caCertPath,omitempty"`
	CAKeyPath  string `json:"caKeyPath,omitempty"`
}

// SandboxSettings configures sandbox behavior.
type SandboxSettings struct {
	// Enabled enables sandbox mode for command execution.
	Enabled bool
	// AutoAllowBashIfSandboxed auto-approves bash commands when sandbox is enabled.
	AutoAllowBashIfSandboxed bool
	// ExcludedCommands are commands that always bypass sandbox restrictions.
	ExcludedCommands []string
	// AllowUnsandboxedCommands allows the model to request running commands outside sandbox.
	AllowUnsandboxedCommands bool
	// Network configures network-specific sandbox settings.
	Network *NetworkSandboxSettings
	// IgnoreViolations configures which sandbox violations to ignore.
	IgnoreViolations *SandboxIgnoreViolations
	// EnableWeakerNestedSandbox enables a weaker nested sandbox for compatibility.
	EnableWeakerNestedSandbox bool
}

// NetworkSandboxSettings configures network-specific sandbox behavior.
type NetworkSandboxSettings struct {
	// AllowLocalBinding allows processes to bind to local ports.
	AllowLocalBinding bool
	// AllowUnixSockets lists Unix socket paths that processes can access.
	AllowUnixSockets []string
	// AllowAllUnixSockets allows access to all Unix sockets.
	AllowAllUnixSockets bool
	// HttpProxyPort is the HTTP proxy port for network requests.
	HttpProxyPort *int
	// SocksProxyPort is the SOCKS proxy port for network requests.
	SocksProxyPort *int
}

// SandboxIgnoreViolations configures which sandbox violations to ignore.
type SandboxIgnoreViolations struct {
	// File lists file path patterns to ignore violations for.
	File []string
	// Network lists network patterns to ignore violations for.
	Network []string
}

// PluginConfig configures a plugin to load.
type PluginConfig struct {
	// Type must be "local" (only local plugins currently supported).
	Type string `json:"type"`
	// Path is the absolute or relative path to the plugin directory.
	Path string `json:"path"`
	// SkipMcpDiscovery, when true, loads skills/hooks/agents/commands from
	// this plugin but does NOT read its .mcp.json or manifest mcpServers.
	// Use when the SDK host owns this plugin's MCP connections.
	SkipMcpDiscovery bool `json:"skipMcpDiscovery,omitempty"`
}

// OutputFormat defines structured output format for agent results.
type OutputFormat struct {
	// Type must be "json_schema".
	Type string
	// Schema is the JSON schema for output validation.
	Schema interface{}
}

// TaskBudget configures the maximum task budget.
type TaskBudget struct {
	Total int `json:"total"`
}

// ToolsConfig configures available tools.
type ToolsConfig struct {
	// Type is "preset" for preset configuration.
	Type string
	// Preset is the preset name (e.g., "claude_code").
	Preset string
	// Tools is a list of specific tool names.
	Tools []string
}

// ThinkingConfig controls Claude's thinking/reasoning behavior.
//
// Type is one of "adaptive", "enabled", or "disabled". BudgetTokens applies only
// when Type is "enabled"; if nil, the CLI is told to use adaptive thinking. Display
// applies when Type is "adaptive" or "enabled" and is one of "summarized" or "omitted".
type ThinkingConfig struct {
	Type         string          `json:"type"`
	BudgetTokens *int            `json:"budgetTokens,omitempty"`
	Display      ThinkingDisplay `json:"display,omitempty"`
}

// ThinkingDisplay controls how thinking blocks are surfaced to the client.
type ThinkingDisplay string

const (
	// ThinkingDisplaySummarized emits a short summary in place of raw thinking.
	ThinkingDisplaySummarized ThinkingDisplay = "summarized"
	// ThinkingDisplayOmitted suppresses thinking blocks entirely.
	ThinkingDisplayOmitted ThinkingDisplay = "omitted"
)

// ThinkingAdaptive lets Claude decide when and how much to think.
func ThinkingAdaptive() *ThinkingConfig {
	return &ThinkingConfig{Type: "adaptive"}
}

// ThinkingEnabled enables thinking with a fixed token budget.
func ThinkingEnabled(budget int) *ThinkingConfig {
	return &ThinkingConfig{
		Type:         "enabled",
		BudgetTokens: &budget,
	}
}

// ThinkingDisabled disables extended thinking.
func ThinkingDisabled() *ThinkingConfig {
	return &ThinkingConfig{Type: "disabled"}
}

// EffortLevel controls how much thinking/reasoning Claude applies.
type EffortLevel string

const (
	// EffortLow applies minimal thinking for fastest responses.
	EffortLow EffortLevel = "low"
	// EffortMedium applies moderate thinking.
	EffortMedium EffortLevel = "medium"
	// EffortHigh applies deep reasoning.
	EffortHigh EffortLevel = "high"
	// EffortXHigh applies deeper reasoning than high. Supported on
	// Opus 4.7+.
	EffortXHigh EffortLevel = "xhigh"
	// EffortMax applies maximum effort. Supported on Opus 4.6+ and
	// Sonnet 4.6.
	EffortMax EffortLevel = "max"
)

// NewOptions creates a new Options with sensible defaults.
//
// Default model is "claude-sonnet-4-5-20250929".
// Default permission mode is PermissionModeDefault.
// Maps are initialized but empty.
func NewOptions() *Options {
	return &Options{
		Model:          "claude-sonnet-4-5-20250929",
		PermissionMode: PermissionModeDefault,
		Env:            make(map[string]string),
		Hooks:          make(map[HookType][]HookConfig),
		Agents:         make(map[string]AgentDefinition),
		MCPServers:     make(map[string]MCPServerConfig),
	}
}

// Option is a functional option for configuring a Client.
type Option func(*Options)

// WithSystemPrompt sets the system prompt sent to Claude.
func WithSystemPrompt(prompt string) Option {
	return func(o *Options) {
		o.SystemPrompt = prompt
	}
}

// WithModel specifies which Claude model to use.
//
// Common models:
// - claude-sonnet-4-5-20250929 (default, best balance)
// - claude-opus-4-5-20250929 (most capable)
// - claude-haiku-4-5-20250929 (fastest, cheapest)
func WithModel(model string) Option {
	return func(o *Options) {
		o.Model = model
	}
}

// WithMainAgent sets the agent to apply to the main thread.
func WithMainAgent(name string) Option {
	return func(o *Options) {
		o.MainAgent = name
	}
}

// WithPlanModeInstructions customizes the plan-mode workflow body.
func WithPlanModeInstructions(instructions string) Option {
	return func(o *Options) {
		o.PlanModeInstructions = instructions
	}
}

// WithTitle sets a custom session title.
func WithTitle(title string) Option {
	return func(o *Options) {
		o.Title = title
	}
}

// WithSkillsAllowlist limits main-session skills to the named allowlist.
func WithSkillsAllowlist(skills []string) Option {
	return func(o *Options) {
		o.Skills = skills
	}
}

// WithPromptSuggestions enables or disables next-prompt suggestion events.
func WithPromptSuggestions(enable bool) Option {
	return func(o *Options) {
		o.PromptSuggestions = &enable
	}
}

// WithAgentProgressSummaries enables or disables agent progress summary events.
func WithAgentProgressSummaries(enable bool) Option {
	return func(o *Options) {
		o.AgentProgressSummaries = &enable
	}
}

// WithForwardSubagentText enables or disables forwarding subagent text.
func WithForwardSubagentText(enable bool) Option {
	return func(o *Options) {
		o.ForwardSubagentText = &enable
	}
}

// WithToolAliases sets the tool-name alias map (see Options.ToolAliases).
func WithToolAliases(aliases map[string]string) Option {
	return func(o *Options) {
		o.ToolAliases = aliases
	}
}

// WithCLIPath sets the path to the Claude Code CLI executable.
//
// If not specified, the CLI will be discovered from the system PATH.
func WithCLIPath(path string) Option {
	return func(o *Options) {
		o.CLIPath = path
	}
}

// WithTransport supplies a custom Transport implementation, bypassing the
// default subprocess. The transport's Connect method will be called by the
// client; do not call it yourself.
func WithTransport(t Transport) Option {
	return func(o *Options) {
		o.Transport = t
	}
}

// WithExtraArgs sets arbitrary Claude CLI flags appended after SDK-managed flags.
func WithExtraArgs(args map[string]*string) Option {
	return func(o *Options) {
		o.ExtraArgs = args
	}
}

// WithEnv adds environment variables for the CLI subprocess.
//
// Use this to set ANTHROPIC_API_KEY if not already in the environment.
func WithEnv(env map[string]string) Option {
	return func(o *Options) {
		if o.Env == nil {
			o.Env = make(map[string]string)
		}
		for k, v := range env {
			o.Env[k] = v
		}
	}
}

// WithPermissionMode sets the permission mode for tool execution.
func WithPermissionMode(mode PermissionMode) Option {
	return func(o *Options) {
		o.PermissionMode = mode
	}
}

// WithCanUseTool sets a callback for runtime permission decisions.
//
// This callback is invoked before each tool execution and can inspect
// the tool name and arguments to make allow/deny decisions.
func WithCanUseTool(fn CanUseToolFunc) Option {
	return func(o *Options) {
		o.CanUseTool = fn
	}
}

// WithOnElicitation registers a callback that handles MCP elicitation requests.
//
// If unset, the SDK auto-declines all elicitation requests.
func WithOnElicitation(fn OnElicitationFunc) Option {
	return func(o *Options) {
		o.OnElicitation = fn
	}
}

// WithOnUserDialog registers a callback that handles request_user_dialog
// control requests.
//
// If unset, the SDK answers every dialog as cancelled and the CLI applies
// each dialog's default behavior.
func WithOnUserDialog(fn OnUserDialogFunc) Option {
	return func(o *Options) {
		o.OnUserDialog = fn
	}
}

// WithSupportedDialogKinds declares the request_user_dialog dialog_kind values
// this consumer can render. Requires WithOnUserDialog; a non-empty list
// without the callback is rejected when the session initializes.
func WithSupportedDialogKinds(kinds ...string) Option {
	return func(o *Options) {
		o.SupportedDialogKinds = kinds
	}
}

// WithPerTaskStopAffordance declares whether this consumer renders a per-task
// stop control, which decides whether an interrupt spares background tasks or
// kills them. See Options.PerTaskStopAffordance.
func WithPerTaskStopAffordance(enabled bool) Option {
	return func(o *Options) {
		o.PerTaskStopAffordance = &enabled
	}
}

// WithGetHostAuthToken registers a callback that refreshes host auth tokens on
// CLI request.
//
// If unset, the SDK responds with an error to any host_auth_token_refresh
// control request.
func WithGetHostAuthToken(fn GetHostAuthTokenFunc) Option {
	return func(o *Options) {
		o.GetHostAuthToken = fn
	}
}

// WithHooks registers lifecycle callbacks.
//
// Example:
//
//	WithHooks(map[HookType][]HookConfig{
//	    HookTypePreToolUse: {
//	        {Matcher: "*", Callback: logToolUse},
//	    },
//	})
func WithHooks(hooks map[HookType][]HookConfig) Option {
	return func(o *Options) {
		o.Hooks = hooks
	}
}

// WithAgents defines specialized subagents for task delegation.
//
// Claude will automatically invoke the appropriate subagent based on
// task context and agent descriptions.
//
// Example:
//
//	WithAgents(map[string]AgentDefinition{
//	    "research": {
//	        Name: "research",
//	        Description: "Research specialist for deep equity analysis",
//	        Prompt: "You are a financial research expert...",
//	        Tools: []string{"fetch_research", "fetch_quote"},
//	    },
//	})
func WithAgents(agents map[string]AgentDefinition) Option {
	return func(o *Options) {
		o.Agents = agents
	}
}

// WithSessionOptions configures session behavior.
//
// Use this to resume existing sessions or fork from a checkpoint.
func WithSessionOptions(opts SessionOptions) Option {
	return func(o *Options) {
		o.SessionOptions = opts
	}
}

// WithResume resumes an existing session by ID.
//
// This is a convenience wrapper around WithSessionOptions.
func WithResume(sessionID string) Option {
	return func(o *Options) {
		o.SessionOptions.Resume = sessionID
	}
}

// WithForkSession creates a branch from an existing session.
//
// This is a convenience wrapper around WithSessionOptions.
func WithForkSession(sessionID string) Option {
	return func(o *Options) {
		o.SessionOptions.ForkFrom = sessionID
	}
}

// WithForkOnResume forks to a new session ID when resuming.
func WithForkOnResume(fork bool) Option {
	return func(o *Options) {
		o.SessionOptions.ForkSession = fork
	}
}

// WithResumeSessionAt resumes a session at a specific message UUID.
func WithResumeSessionAt(messageUUID string) Option {
	return func(o *Options) {
		o.SessionOptions.ResumeSessionAt = messageUUID
	}
}

// WithResumeDropsTurn arms the fork-point guard for a truncating
// WithResumeSessionAt resume: messageUUID is the prompt UUID of the turn being
// discarded, and the CLI refuses the resume if anything else falls in the
// discarded range. See SessionOptions.ResumeDropsTurn for the refusal contract.
func WithResumeDropsTurn(messageUUID string) Option {
	return func(o *Options) {
		o.SessionOptions.ResumeDropsTurn = messageUUID
	}
}

// WithMCPServers configures MCP servers for custom tool integration.
func WithMCPServers(servers map[string]MCPServerConfig) Option {
	return func(o *Options) {
		o.MCPServers = servers
	}
}

// WithMcpServer adds an in-process MCP server.
//
// In-process MCP servers run within the SDK process. Tool calls are routed
// through the control channel rather than spawning separate processes.
// This is useful for defining custom tools without building separate binaries.
//
// Example:
//
//	server := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{
//	    Name: "calculator",
//	})
//	claudeagent.AddTool(server, claudeagent.ToolDef{
//	    Name:        "add",
//	    Description: "Add two numbers",
//	}, addHandler)
//
//	client, _ := claudeagent.NewClient(
//	    claudeagent.WithMcpServer("calculator", server),
//	)
func WithMcpServer(name string, server *McpServer) Option {
	return func(o *Options) {
		if o.SDKMcpServers == nil {
			o.SDKMcpServers = make(map[string]*McpServer)
		}
		o.SDKMcpServers[name] = server
	}
}

// WithVerbose enables debug logging from the CLI.
func WithVerbose(verbose bool) Option {
	return func(o *Options) {
		o.Verbose = verbose
	}
}

// WithAskUserQuestionHandler sets a callback to handle user questions.
//
// When Claude invokes the AskUserQuestion tool, this handler is called
// with the question set. The handler should return answers using the
// QuestionSet helper methods.
//
// If no handler is set, questions are routed to the Questions() iterator
// on the client.
//
// Example:
//
//	WithAskUserQuestionHandler(func(ctx context.Context, qs QuestionSet) (Answers, error) {
//	    // Auto-select first option for first question
//	    return qs.Answer(0, qs.Questions[0].Options[0].Label), nil
//	})
func WithAskUserQuestionHandler(handler AskUserQuestionHandler) Option {
	return func(o *Options) {
		o.AskUserQuestionHandler = handler
	}
}

// PermissionMode controls how tool execution permissions are handled.
type PermissionMode string

const (
	// PermissionModeDefault uses standard permission checks.
	PermissionModeDefault PermissionMode = "default"

	// PermissionModePlan is planning mode (no tool execution).
	PermissionModePlan PermissionMode = "plan"

	// PermissionModeAcceptEdits auto-approves file operations.
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"

	// PermissionModeBypassAll skips all permission checks.
	PermissionModeBypassAll PermissionMode = "bypassPermissions"

	// PermissionModeAuto lets Claude automatically decide permission handling.
	PermissionModeAuto PermissionMode = "auto"

	// PermissionModeDontAsk runs without asking for permission prompts.
	PermissionModeDontAsk PermissionMode = "dontAsk"
)

// CanUseToolFunc is a callback invoked before tool execution.
//
// Return PermissionAllow{} to proceed or PermissionDeny{Reason: "..."} to block.
type CanUseToolFunc func(ctx context.Context, req ToolPermissionRequest) PermissionResult

// OnElicitationFunc is invoked when an MCP server requests user input.
//
// Returning a non-nil error converts the response to action="cancel".
type OnElicitationFunc func(ctx context.Context, req ElicitationRequest) (ElicitationResult, error)

// OnUserDialogFunc handles a request_user_dialog control request.
//
// Returning a non-nil error or a UserDialogResult with
// Behavior == UserDialogBehaviorCancelled signals the CLI to apply the
// dialog's default behavior. Hosts MUST answer with cancelled for any
// dialogKind they do not recognize.
type OnUserDialogFunc func(ctx context.Context, req UserDialogRequest) (UserDialogResult, error)

// GetHostAuthTokenFunc returns a fresh host auth token.
//
// The context is canceled when the CLI cancels the refresh request.
type GetHostAuthTokenFunc func(ctx context.Context) (string, error)

// ElicitationRequest is the input to the OnElicitation callback.
type ElicitationRequest struct {
	ServerName      string                 `json:"serverName"`
	Message         string                 `json:"message"`
	Mode            string                 `json:"mode,omitempty"`
	URL             string                 `json:"url,omitempty"`
	ElicitationID   string                 `json:"elicitationId,omitempty"`
	RequestedSchema map[string]interface{} `json:"requestedSchema,omitempty"`
	Title           string                 `json:"title,omitempty"`
	DisplayName     string                 `json:"displayName,omitempty"`
	Description     string                 `json:"description,omitempty"`
}

// ElicitationResult is what OnElicitation returns.
type ElicitationResult struct {
	Action  string                 `json:"action"`
	Content map[string]interface{} `json:"content,omitempty"`
}

const (
	// ElicitationActionAccept accepts the elicitation response.
	ElicitationActionAccept = "accept"
	// ElicitationActionDecline declines the elicitation response.
	ElicitationActionDecline = "decline"
	// ElicitationActionCancel cancels the elicitation response.
	ElicitationActionCancel = "cancel"
)

// UserDialogRequest is the input to the OnUserDialog callback.
//
// DialogKind is an open string union — new kinds may appear without a
// protocol bump. Hosts MUST answer unrecognized kinds with
// UserDialogBehaviorCancelled so the CLI applies the dialog default.
type UserDialogRequest struct {
	DialogKind string                 `json:"dialogKind"`
	Payload    map[string]interface{} `json:"payload"`
	ToolUseID  string                 `json:"toolUseID,omitempty"`
}

// UserDialogResult is what OnUserDialog returns. The TS wire spec is a
// discriminated union with only two valid Behavior values:
//
//   - UserDialogBehaviorCompleted — Result carries the host's answer
//     (dialog-kind-specific shape, transported opaquely).
//   - UserDialogBehaviorCancelled — the host declined to answer; the CLI
//     applies the dialog's default behavior. Result MUST be nil.
type UserDialogResult struct {
	Behavior string      `json:"behavior"`
	Result   interface{} `json:"result,omitempty"`
}

const (
	// UserDialogBehaviorCompleted signals the host produced an answer.
	UserDialogBehaviorCompleted = "completed"
	// UserDialogBehaviorCancelled signals the CLI to apply the dialog's
	// default behavior.
	UserDialogBehaviorCancelled = "cancelled"
)

// ToolPermissionRequest contains details about a tool execution request.
type ToolPermissionRequest struct {
	ToolName  string          // Tool identifier (e.g., "mcp__tickertape__fetch_quote")
	Arguments json.RawMessage // Tool arguments as JSON
	Context   PermissionContext
}

// MatchedAskRule describes the user-configured ask rule (permissions.ask) that
// forced a permission prompt whose ask nonetheless carries the tool's own
// decision reason. When set, the ask-rule substitution kept the richer
// tool-minted ask, so the rule rides here instead of a "rule"
// decision_reason_type. Hosts making policy on the decision reason (e.g.
// auto-deny a safetyCheck) or running host-side auto-approval should treat asks
// carrying this as rule-forced: the user's stated intent is a human prompt. The
// values are producer-authored and render-unsafe like a decision reason —
// sanitize before display (sdk.d.ts v0.3.215).
type MatchedAskRule struct {
	Source      string `json:"source"`
	ToolName    string `json:"tool_name"`
	RuleContent string `json:"rule_content,omitempty"`
}

// PermissionContext provides additional context for permission decisions.
type PermissionContext struct {
	SessionID string
	ToolUseID string
	AgentID   string
	// RequiresUserInteraction is true when the tool's approval card is
	// itself the user-interaction surface (Tool.requiresUserInteraction()).
	// SDK hosts must not offer a one-tap allow/deny for these — the user has
	// to open the session and respond on the card itself (sdk.d.ts
	// v0.3.201). As of v0.3.215 it is also set when the pending ask is
	// localDisplayOnly: its consent disclosure cannot ride this wire and only
	// the local dialog renders it. Either way the user must open the session
	// to answer.
	RequiresUserInteraction bool
	// SuppressAlwaysAllowRule is true when the dialog must not offer the
	// persistent "don't ask again" affordance for this ask: accepting it would
	// write a whole-tool allow rule broader than the ask's own verb. Hosts
	// rendering approve options should omit any persistent-rule row when set
	// (sdk.d.ts v0.3.215).
	SuppressAlwaysAllowRule bool
	// MatchedAskRule is set when a user-configured ask rule forced this prompt
	// while the ask still carries the tool's own decision reason. Nil when no
	// such rule applied. See MatchedAskRule (sdk.d.ts v0.3.215).
	MatchedAskRule *MatchedAskRule
	Metadata       map[string]interface{}
}

// PermissionDecisionClassification labels how a permission decision was reached
// for SDK-host telemetry. If unset, the CLI infers conservatively.
type PermissionDecisionClassification string

const (
	// PermissionClassificationUserTemporary is "allow-once".
	PermissionClassificationUserTemporary PermissionDecisionClassification = "user_temporary"
	// PermissionClassificationUserPermanent is "always-allow" and later cache hits.
	PermissionClassificationUserPermanent PermissionDecisionClassification = "user_permanent"
	// PermissionClassificationUserReject is "deny".
	PermissionClassificationUserReject PermissionDecisionClassification = "user_reject"
)

// PermissionResult is the outcome of a permission check.
type PermissionResult interface {
	IsAllow() bool
}

// PermissionAllow indicates permission granted.
type PermissionAllow struct {
	// Classification optionally labels the decision for telemetry. Empty = unset.
	Classification PermissionDecisionClassification
}

// IsAllow implements PermissionResult.
func (PermissionAllow) IsAllow() bool { return true }

// PermissionDeny indicates permission denied.
type PermissionDeny struct {
	Reason string
	// Classification optionally labels the decision for telemetry. Empty = unset.
	Classification PermissionDecisionClassification
}

// IsAllow implements PermissionResult.
func (PermissionDeny) IsAllow() bool { return false }

// HookType identifies a lifecycle event.
type HookType string

const (
	// HookTypeConfigChange fires when configuration changes.
	HookTypeConfigChange HookType = "ConfigChange"

	// HookTypeInstructionsLoaded fires when instruction files are loaded.
	HookTypeInstructionsLoaded HookType = "InstructionsLoaded"

	// HookTypeMessageDisplay fires as assistant message display lines stream.
	HookTypeMessageDisplay HookType = "MessageDisplay"

	// HookTypePreToolUse fires before tool execution.
	HookTypePreToolUse HookType = "PreToolUse"

	// HookTypePostToolUse fires after tool execution.
	HookTypePostToolUse HookType = "PostToolUse"

	// HookTypePostToolUseFailure fires when tool execution fails.
	HookTypePostToolUseFailure HookType = "PostToolUseFailure"

	// HookTypeNotification fires when Claude sends notifications.
	HookTypeNotification HookType = "Notification"

	// HookTypeUserPromptSubmit fires when a user message is submitted.
	HookTypeUserPromptSubmit HookType = "UserPromptSubmit"

	// HookTypeSessionStart fires when a session starts.
	HookTypeSessionStart HookType = "SessionStart"

	// HookTypeSessionEnd fires when a session ends.
	HookTypeSessionEnd HookType = "SessionEnd"

	// HookTypeStop fires when a session is stopping.
	HookTypeStop HookType = "Stop"

	// HookTypeSubagentStart fires when a subagent starts.
	HookTypeSubagentStart HookType = "SubagentStart"

	// HookTypeSubagentStop fires when a subagent finishes.
	HookTypeSubagentStop HookType = "SubagentStop"

	// HookTypePreCompact fires before context compaction.
	HookTypePreCompact HookType = "PreCompact"

	// HookTypePostCompact fires after context compaction.
	HookTypePostCompact HookType = "PostCompact"

	// HookTypePreModelSwitch fires before the active model changes, and can
	// veto the switch via HookResult.PermissionDecision.
	HookTypePreModelSwitch HookType = "PreModelSwitch"

	// HookTypePostModelSwitch fires after the active model has changed.
	HookTypePostModelSwitch HookType = "PostModelSwitch"

	// HookTypePostToolBatch fires after a batch of tool calls completes.
	HookTypePostToolBatch HookType = "PostToolBatch"

	// HookTypePermissionRequest fires when permission check requested.
	HookTypePermissionRequest HookType = "PermissionRequest"

	// HookTypePermissionDenied fires when permission is denied.
	HookTypePermissionDenied HookType = "PermissionDenied"

	// HookTypeCwdChanged fires when the current working directory changes.
	HookTypeCwdChanged HookType = "CwdChanged"

	// HookTypeDirectoryAdded fires when a directory is added as a
	// working-directory root (via /add-dir or the register_repo_root control
	// request).
	HookTypeDirectoryAdded HookType = "DirectoryAdded"

	// HookTypeFileChanged fires when a watched file changes.
	HookTypeFileChanged HookType = "FileChanged"

	// HookTypeElicitation fires when an MCP server requests elicitation.
	HookTypeElicitation HookType = "Elicitation"

	// HookTypeElicitationResult fires when an elicitation response is available.
	HookTypeElicitationResult HookType = "ElicitationResult"

	// HookTypeSetup fires during setup.
	HookTypeSetup HookType = "Setup"

	// HookTypeStopFailure fires when a stop attempt fails.
	HookTypeStopFailure HookType = "StopFailure"

	// HookTypeTaskCompleted fires when a task completes.
	HookTypeTaskCompleted HookType = "TaskCompleted"

	// HookTypeTaskCreated fires when a task is created.
	HookTypeTaskCreated HookType = "TaskCreated"

	// HookTypeTeammateIdle fires when a teammate becomes idle.
	HookTypeTeammateIdle HookType = "TeammateIdle"

	// HookTypeUserPromptExpansion fires when a prompt expansion occurs.
	HookTypeUserPromptExpansion HookType = "UserPromptExpansion"

	// HookTypeWorktreeCreate fires when a worktree is created.
	HookTypeWorktreeCreate HookType = "WorktreeCreate"

	// HookTypeWorktreeRemove fires when a worktree is removed.
	HookTypeWorktreeRemove HookType = "WorktreeRemove"
)

// HookConfig defines a lifecycle callback.
type HookConfig struct {
	Type     HookType     // Hook event type
	Matcher  string       // Glob pattern for tool names (e.g., "*", "fetch_*")
	Timeout  int          // Optional timeout in seconds; 0 = use default
	Callback HookCallback // Callback function
}

// HookCallback is invoked when a hook event fires.
//
// The callback can inspect and modify arguments/results via the HookResult.
type HookCallback func(ctx context.Context, input HookInput) (HookResult, error)

// HookInput is the base interface for hook inputs.
type HookInput interface {
	HookType() HookType
	Base() BaseHookInput
}

// HookEffort is the shape of BaseHookInput.effort: the reasoning effort
// applied to the current turn, after any silent downgrade for the selected
// model.
type HookEffort struct {
	// Level is the active effort level for the current turn
	// (e.g., "low", "medium", "high", "xhigh", "max"). Also exposed to
	// hook commands and Bash as the CLAUDE_EFFORT env var.
	Level EffortLevel `json:"level"`
}

// BaseHookInput contains common fields for all hook inputs.
type BaseHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	// PromptID is a UUID correlating a user prompt with all subsequent
	// events until the next prompt. The same value is emitted on
	// OpenTelemetry events as the prompt.id attribute, so hook output can
	// be joined to OTel events at prompt grain. Absent until the first user
	// input of the process lifetime (sdk.d.ts v0.3.201).
	PromptID       string `json:"prompt_id,omitempty"`
	PermissionMode string `json:"permission_mode,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	AgentType      string `json:"agent_type,omitempty"`
	// Effort is the reasoning effort applied to the current turn. Present for
	// hooks that fire within a tool-use context (PreToolUse, PostToolUse, Stop,
	// SubagentStop, etc.) on a model that supports the effort parameter; absent
	// for session-lifecycle hooks and models without effort support.
	Effort *HookEffort `json:"effort,omitempty"`
}

// ConfigChangeInput contains data for ConfigChange hooks.
type ConfigChangeInput struct {
	BaseHookInput
	Source   string `json:"source"`
	FilePath string `json:"file_path,omitempty"`
}

// HookType implements HookInput.
func (ConfigChangeInput) HookType() HookType { return HookTypeConfigChange }

// Base implements HookInput.
func (i ConfigChangeInput) Base() BaseHookInput { return i.BaseHookInput }

// InstructionsLoadedInput contains data for InstructionsLoaded hooks.
type InstructionsLoadedInput struct {
	BaseHookInput
	FilePath        string   `json:"file_path"`
	MemoryType      string   `json:"memory_type"`
	LoadReason      string   `json:"load_reason"`
	Globs           []string `json:"globs,omitempty"`
	TriggerFilePath string   `json:"trigger_file_path,omitempty"`
	ParentFilePath  string   `json:"parent_file_path,omitempty"`
}

// HookType implements HookInput.
func (InstructionsLoadedInput) HookType() HookType { return HookTypeInstructionsLoaded }

// Base implements HookInput.
func (i InstructionsLoadedInput) Base() BaseHookInput { return i.BaseHookInput }

// MessageDisplayInput contains data for MessageDisplay hooks per sdk.d.ts
// v0.3.168 L1153-L1180.
type MessageDisplayInput struct {
	BaseHookInput
	TurnID    string `json:"turn_id"`
	MessageID string `json:"message_id"`
	Index     int    `json:"index"`
	Final     bool   `json:"final"`
	Delta     string `json:"delta"`
}

// HookType implements HookInput.
func (MessageDisplayInput) HookType() HookType { return HookTypeMessageDisplay }

// Base implements HookInput.
func (i MessageDisplayInput) Base() BaseHookInput { return i.BaseHookInput }

// PreToolUseInput contains data for PreToolUse hooks.
type PreToolUseInput struct {
	BaseHookInput
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// HookType implements HookInput.
func (PreToolUseInput) HookType() HookType { return HookTypePreToolUse }

// Base implements HookInput.
func (i PreToolUseInput) Base() BaseHookInput { return i.BaseHookInput }

// PostToolUseInput contains data for PostToolUse hooks.
type PostToolUseInput struct {
	BaseHookInput
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
}

// HookType implements HookInput.
func (PostToolUseInput) HookType() HookType { return HookTypePostToolUse }

// Base implements HookInput.
func (i PostToolUseInput) Base() BaseHookInput { return i.BaseHookInput }

// UserPromptSubmitInput contains data for UserPromptSubmit hooks.
type UserPromptSubmitInput struct {
	BaseHookInput
	Prompt string `json:"prompt"`
	// Source names who authored or injected the prompt: "user" (interactive
	// composer), "sdk" (non-interactive entrypoint, -p / Agent SDK),
	// "loop_wakeup" (dynamic /loop wakeup), "schedule_wakeup" (scheduled-task
	// fire), "system" (other machine-injected turns: peer/channel messages,
	// task notifications, auto-continuation), or "poll_event" (the poll-event
	// channel enqueue-time pass, added in sdk.d.ts v0.3.241 L8228).
	//
	// A "poll_event" pass is the one source where a blocking verdict rejects
	// something that has not been delivered yet: the hook fires when the host
	// submits the event, before its delivery ack exists.
	//
	// Empty when absent — as of v0.3.215 it is only set for
	// Anthropic-internal sessions while the field is trialed; external
	// payloads omit it (sdk.d.ts v0.3.215).
	Source string `json:"source,omitempty"`
	// SessionTitle is the optional user-facing session label from TS L6094.
	// Nil means the field was absent on the wire.
	SessionTitle *string `json:"session_title,omitempty"`
}

// HookType implements HookInput.
func (UserPromptSubmitInput) HookType() HookType { return HookTypeUserPromptSubmit }

// Base implements HookInput.
func (i UserPromptSubmitInput) Base() BaseHookInput { return i.BaseHookInput }

// BackgroundTaskSummary is a snapshot of an in-flight background task
// registered in the session.
type BackgroundTaskSummary struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
	Server      string `json:"server,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Name        string `json:"name,omitempty"`
}

// SessionCronSummary is a snapshot of a session-scoped cron task that will
// wake this session later (CronCreate, ScheduleWakeup, /loop).
type SessionCronSummary struct {
	ID        string `json:"id"`
	Schedule  string `json:"schedule"`
	Recurring bool   `json:"recurring"`
	Prompt    string `json:"prompt"`
}

// StopInput contains data for Stop hooks.
type StopInput struct {
	BaseHookInput
	StopHookActive       bool                    `json:"stop_hook_active"`
	LastAssistantMessage string                  `json:"last_assistant_message,omitempty"`
	BackgroundTasks      []BackgroundTaskSummary `json:"background_tasks,omitempty"`
	SessionCrons         []SessionCronSummary    `json:"session_crons,omitempty"`
}

// HookType implements HookInput.
func (StopInput) HookType() HookType { return HookTypeStop }

// Base implements HookInput.
func (i StopInput) Base() BaseHookInput { return i.BaseHookInput }

// SubagentStopInput contains data for SubagentStop hooks.
//
// AgentID and AgentType live on the embedded BaseHookInput (TS treats them as
// base hook fields) — read them via i.Base() or i.BaseHookInput.AgentID.
type SubagentStopInput struct {
	BaseHookInput
	AgentName            string                  `json:"agent_name"`
	Status               string                  `json:"status"`
	Result               string                  `json:"result"`
	StopHookActive       bool                    `json:"stop_hook_active"`
	AgentTranscriptPath  string                  `json:"agent_transcript_path,omitempty"`
	LastAssistantMessage string                  `json:"last_assistant_message,omitempty"`
	BackgroundTasks      []BackgroundTaskSummary `json:"background_tasks,omitempty"`
	SessionCrons         []SessionCronSummary    `json:"session_crons,omitempty"`
}

// HookType implements HookInput.
func (SubagentStopInput) HookType() HookType { return HookTypeSubagentStop }

// Base implements HookInput.
func (i SubagentStopInput) Base() BaseHookInput { return i.BaseHookInput }

// PreCompactInput contains data for PreCompact hooks.
type PreCompactInput struct {
	BaseHookInput
	Trigger            string  `json:"trigger"` // "manual" or "auto"
	CustomInstructions *string `json:"custom_instructions,omitempty"`
	MessageCount       int     `json:"message_count"`
}

// HookType implements HookInput.
func (PreCompactInput) HookType() HookType { return HookTypePreCompact }

// Base implements HookInput.
func (i PreCompactInput) Base() BaseHookInput { return i.BaseHookInput }

// PostCompactInput contains data for PostCompact hooks.
type PostCompactInput struct {
	BaseHookInput
	Trigger        string `json:"trigger"`
	CompactSummary string `json:"compact_summary"`
}

// HookType implements HookInput.
func (PostCompactInput) HookType() HookType { return HookTypePostCompact }

// Base implements HookInput.
func (i PostCompactInput) Base() BaseHookInput { return i.BaseHookInput }

// CacheTTL is the lifetime of a prompt-cache entry. It bounds how long a warm
// cache stays warm, and so how much of a model switch or a session resume can
// be served without paying a fresh cache write.
type CacheTTL string

const (
	// CacheTTL5m is the default prompt-cache lifetime.
	CacheTTL5m CacheTTL = "5m"

	// CacheTTL1h is the extended prompt-cache lifetime.
	CacheTTL1h CacheTTL = "1h"
)

// ModelSwitchSource identifies what initiated a model switch.
type ModelSwitchSource string

const (
	// ModelSwitchSourceCommand is a switch from the /model slash command.
	ModelSwitchSourceCommand ModelSwitchSource = "command"

	// ModelSwitchSourcePicker is a switch from the interactive model picker.
	ModelSwitchSourcePicker ModelSwitchSource = "picker"

	// ModelSwitchSourceSDK is a switch driven by the SDK's set_model control
	// request.
	ModelSwitchSourceSDK ModelSwitchSource = "sdk"

	// ModelSwitchSourceAuto is a switch the CLI performed on its own, such as
	// a fallback after a refusal. PostModelSwitch only — see
	// PostModelSwitchInput.Source.
	ModelSwitchSourceAuto ModelSwitchSource = "auto"

	// ModelSwitchSourceResume is the model being restored when a session is
	// resumed. PostModelSwitch only — see PostModelSwitchInput.Source.
	ModelSwitchSourceResume ModelSwitchSource = "resume"
)

// ModelPricingBasis says where the per-token prices behind a cost estimate came
// from.
type ModelPricingBasis string

const (
	// ModelPricingConfigured means Settings.ModelPricing supplied the rates.
	ModelPricingConfigured ModelPricingBasis = "configured"

	// ModelPricingCatalog means the rates came from the published model
	// catalog.
	ModelPricingCatalog ModelPricingBasis = "catalog"

	// ModelPricingDefault means neither applied and a fallback rate was used,
	// so the estimate is indicative only.
	ModelPricingDefault ModelPricingBasis = "default"
)

// modelSwitchContext is the prompt-cache economics both model-switch hooks
// carry. A switch invalidates the warm cache for the outgoing model, so the
// interesting question at hook time is what re-warming will cost.
type modelSwitchContext struct {
	// FromModel is the model in effect before the switch.
	FromModel string `json:"from_model"`

	// ToModel is the model that will be (or has been) switched to.
	ToModel string `json:"to_model"`

	// RequestedModel is what the user or caller actually asked for, when that
	// differs from the resolved ToModel. Nil when the request named no target
	// — cycling the picker, say — and also on CLIs that predate the field;
	// the two are not distinguishable and nothing consumes the difference.
	RequestedModel *string `json:"requested_model,omitempty"`

	// ContextTokens is the size of the context that would have to be re-sent
	// against the new model.
	ContextTokens int `json:"context_tokens"`

	// PromptCacheWarm reports whether the outgoing model's prompt cache is
	// currently warm. Switching away from a warm cache is what makes
	// EstimatedCacheWriteUSD non-trivial.
	PromptCacheWarm bool `json:"prompt_cache_warm"`

	// CacheTTL is the prompt-cache lifetime in effect.
	CacheTTL CacheTTL `json:"cache_ttl"`

	// EstimatedCacheWriteUSD is the projected cost of writing the context into
	// the new model's cache.
	EstimatedCacheWriteUSD float64 `json:"estimated_cache_write_usd"`

	// Pricing says where the rates behind EstimatedCacheWriteUSD came from.
	// ModelPricingDefault means treat the figure as indicative.
	Pricing ModelPricingBasis `json:"pricing"`
}

// PreModelSwitchInput contains data for PreModelSwitch hooks.
//
// The hook can veto the switch: returning HookResult.PermissionDecision "deny"
// cancels it, "ask" asks the user to confirm (a headless session refuses
// instead), and "allow" proceeds while skipping the interactive cache-miss
// confirm (sdk.d.ts v0.3.251 L2482).
type PreModelSwitchInput struct {
	BaseHookInput
	modelSwitchContext

	// Source is what initiated the switch: ModelSwitchSourceCommand,
	// ModelSwitchSourcePicker, or ModelSwitchSourceSDK. A switch the CLI made
	// on its own has no vetoable pre-phase and so never reaches this hook.
	Source ModelSwitchSource `json:"source"`
}

// HookType implements HookInput.
func (PreModelSwitchInput) HookType() HookType { return HookTypePreModelSwitch }

// Base implements HookInput.
func (i PreModelSwitchInput) Base() BaseHookInput { return i.BaseHookInput }

// PostModelSwitchInput contains data for PostModelSwitch hooks.
type PostModelSwitchInput struct {
	BaseHookInput
	modelSwitchContext

	// Source is what initiated the switch. Beyond the three values
	// PreModelSwitch admits, this also reports ModelSwitchSourceAuto and
	// ModelSwitchSourceResume — switches the CLI performed itself, which by
	// construction only ever surface after the fact.
	Source ModelSwitchSource `json:"source"`
}

// HookType implements HookInput.
func (PostModelSwitchInput) HookType() HookType { return HookTypePostModelSwitch }

// Base implements HookInput.
func (i PostModelSwitchInput) Base() BaseHookInput { return i.BaseHookInput }

// PostToolBatchInput contains data for PostToolBatch hooks.
type PostToolBatchInput struct {
	BaseHookInput
	ToolCalls []PostToolBatchToolCall `json:"tool_calls"`
}

// PostToolBatchToolCall contains one tool call result from a PostToolBatch hook.
type PostToolBatchToolCall struct {
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolUseID    string          `json:"tool_use_id"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
}

// HookType implements HookInput.
func (PostToolBatchInput) HookType() HookType { return HookTypePostToolBatch }

// Base implements HookInput.
func (i PostToolBatchInput) Base() BaseHookInput { return i.BaseHookInput }

// PostToolUseFailureInput contains data for PostToolUseFailure hooks.
type PostToolUseFailureInput struct {
	BaseHookInput
	ToolName    string          `json:"tool_name"`
	ToolInput   json.RawMessage `json:"tool_input"`
	Error       string          `json:"error"`
	IsInterrupt bool            `json:"is_interrupt,omitempty"`
}

// HookType implements HookInput.
func (PostToolUseFailureInput) HookType() HookType { return HookTypePostToolUseFailure }

// Base implements HookInput.
func (i PostToolUseFailureInput) Base() BaseHookInput { return i.BaseHookInput }

// NotificationInput contains data for Notification hooks.
type NotificationInput struct {
	BaseHookInput
	Message string `json:"message"`
	Title   string `json:"title,omitempty"`
}

// HookType implements HookInput.
func (NotificationInput) HookType() HookType { return HookTypeNotification }

// Base implements HookInput.
func (i NotificationInput) Base() BaseHookInput { return i.BaseHookInput }

// SessionStartInput contains data for SessionStart hooks.
type SessionStartInput struct {
	BaseHookInput
	Source string `json:"source"` // "startup", "resume", "clear", "compact", or "fork" (v0.3.215)
	// SessionTitle is the optional user-facing session label from TS L3978.
	// Nil means the field was absent on the wire.
	SessionTitle *string `json:"session_title,omitempty"`
}

// HookType implements HookInput.
func (SessionStartInput) HookType() HookType { return HookTypeSessionStart }

// Base implements HookInput.
func (i SessionStartInput) Base() BaseHookInput { return i.BaseHookInput }

// SessionEndInput contains data for SessionEnd hooks.
type SessionEndInput struct {
	BaseHookInput
	Reason string `json:"reason"` // Exit reason
}

// HookType implements HookInput.
func (SessionEndInput) HookType() HookType { return HookTypeSessionEnd }

// Base implements HookInput.
func (i SessionEndInput) Base() BaseHookInput { return i.BaseHookInput }

// SubagentStartInput contains data for SubagentStart hooks.
type SubagentStartInput struct {
	BaseHookInput
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
}

// HookType implements HookInput.
func (SubagentStartInput) HookType() HookType { return HookTypeSubagentStart }

// Base implements HookInput.
func (i SubagentStartInput) Base() BaseHookInput { return i.BaseHookInput }

// PermissionRequestInput contains data for PermissionRequest hooks.
type PermissionRequestInput struct {
	BaseHookInput
	ToolName              string             `json:"tool_name"`
	ToolInput             json.RawMessage    `json:"tool_input"`
	PermissionSuggestions []PermissionUpdate `json:"permission_suggestions,omitempty"`
}

// HookType implements HookInput.
func (PermissionRequestInput) HookType() HookType { return HookTypePermissionRequest }

// Base implements HookInput.
func (i PermissionRequestInput) Base() BaseHookInput { return i.BaseHookInput }

// PermissionDeniedInput contains data for PermissionDenied hooks.
type PermissionDeniedInput struct {
	BaseHookInput
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	ToolUseID string          `json:"tool_use_id"`
	Reason    string          `json:"reason"`
}

// HookType implements HookInput.
func (PermissionDeniedInput) HookType() HookType { return HookTypePermissionDenied }

// Base implements HookInput.
func (i PermissionDeniedInput) Base() BaseHookInput { return i.BaseHookInput }

// CwdChangedInput contains data for CwdChanged hooks.
type CwdChangedInput struct {
	BaseHookInput
	OldCwd string `json:"old_cwd"`
	NewCwd string `json:"new_cwd"`
}

// HookType implements HookInput.
func (CwdChangedInput) HookType() HookType { return HookTypeCwdChanged }

// Base implements HookInput.
func (i CwdChangedInput) Base() BaseHookInput { return i.BaseHookInput }

// DirectoryAddedInput contains data for DirectoryAdded hooks. It fires when a
// directory is registered as a working-directory root. Mirrors
// DirectoryAddedHookInput in sdk.d.ts v0.3.220 L532.
type DirectoryAddedInput struct {
	BaseHookInput
	// Directory is the absolute path of the directory that was added.
	Directory string `json:"directory"`
	// Source is how the directory was added: "slash_command" for /add-dir,
	// "register_repo_root" for the SDK control_request.
	Source string `json:"source"`
}

// HookType implements HookInput.
func (DirectoryAddedInput) HookType() HookType { return HookTypeDirectoryAdded }

// Base implements HookInput.
func (i DirectoryAddedInput) Base() BaseHookInput { return i.BaseHookInput }

// FileChangedInput contains data for FileChanged hooks.
type FileChangedInput struct {
	BaseHookInput
	FilePath string `json:"file_path"`
	Event    string `json:"event"`
}

// HookType implements HookInput.
func (FileChangedInput) HookType() HookType { return HookTypeFileChanged }

// Base implements HookInput.
func (i FileChangedInput) Base() BaseHookInput { return i.BaseHookInput }

// ElicitationInput contains data for Elicitation hooks.
type ElicitationInput struct {
	BaseHookInput
	MCPServerName   string                 `json:"mcp_server_name"`
	Message         string                 `json:"message"`
	Mode            string                 `json:"mode,omitempty"`
	URL             string                 `json:"url,omitempty"`
	ElicitationID   string                 `json:"elicitation_id,omitempty"`
	RequestedSchema map[string]interface{} `json:"requested_schema,omitempty"`
}

// HookType implements HookInput.
func (ElicitationInput) HookType() HookType { return HookTypeElicitation }

// Base implements HookInput.
func (i ElicitationInput) Base() BaseHookInput { return i.BaseHookInput }

// ElicitationResultInput contains data for ElicitationResult hooks.
type ElicitationResultInput struct {
	BaseHookInput
	MCPServerName string                 `json:"mcp_server_name"`
	ElicitationID string                 `json:"elicitation_id,omitempty"`
	Mode          string                 `json:"mode,omitempty"`
	Action        string                 `json:"action"`
	Content       map[string]interface{} `json:"content,omitempty"`
}

// HookType implements HookInput.
func (ElicitationResultInput) HookType() HookType { return HookTypeElicitationResult }

// Base implements HookInput.
func (i ElicitationResultInput) Base() BaseHookInput { return i.BaseHookInput }

// SetupInput contains data for Setup hooks.
type SetupInput struct {
	BaseHookInput
	Trigger string `json:"trigger"`
}

// HookType implements HookInput.
func (SetupInput) HookType() HookType { return HookTypeSetup }

// Base implements HookInput.
func (i SetupInput) Base() BaseHookInput { return i.BaseHookInput }

// AssistantMessageError identifies an assistant message error code.
type AssistantMessageError string

const (
	AssistantMessageErrorAuthenticationFailed AssistantMessageError = "authentication_failed"
	AssistantMessageErrorOAuthOrgNotAllowed   AssistantMessageError = "oauth_org_not_allowed"
	// AssistantMessageErrorAccountOnHold marks a turn refused because the
	// account is on hold (sdk.d.ts v0.3.241 L159).
	AssistantMessageErrorAccountOnHold   AssistantMessageError = "account_on_hold"
	AssistantMessageErrorBillingError    AssistantMessageError = "billing_error"
	AssistantMessageErrorRateLimit       AssistantMessageError = "rate_limit"
	AssistantMessageErrorOverloaded      AssistantMessageError = "overloaded"
	AssistantMessageErrorInvalidRequest  AssistantMessageError = "invalid_request"
	AssistantMessageErrorModelNotFound   AssistantMessageError = "model_not_found"
	AssistantMessageErrorServerError     AssistantMessageError = "server_error"
	AssistantMessageErrorUnknown         AssistantMessageError = "unknown"
	AssistantMessageErrorMaxOutputTokens AssistantMessageError = "max_output_tokens"
)

// Known values for the apiKeySource field on SystemMessage and AccountInfo,
// naming where the credential used for API requests came from.
//
// The field stays a plain string: it is an open set on the wire, and the CLI
// is free to add sources. Compare against these instead of literals, but do
// not assume the set is exhaustive.
//
// TS also retains five legacy members — "user", "project", "org", "temporary"
// and "oauth" — solely so the type stays backward compatible. Current CLIs
// never emit them, so there is nothing to branch on and they get no constants
// here (sdk.d.ts v0.3.241 L124).
const (
	// APIKeySourceAnthropicAPIKey means the ANTHROPIC_API_KEY environment
	// variable supplied the credential.
	// These name where a credential came from; they are not credentials.
	APIKeySourceAnthropicAPIKey = "ANTHROPIC_API_KEY" // #nosec G101 // #nosec G101
	// APIKeySourceAPIKeyHelper means the configured apiKeyHelper command
	// supplied the credential.
	APIKeySourceAPIKeyHelper = "apiKeyHelper" // #nosec G101
	// APIKeySourceLoginManagedKey means the credential is an API key created
	// and stored by /login with an Anthropic Console account.
	APIKeySourceLoginManagedKey = "/login managed key" // #nosec G101
	// APIKeySourceNone means no API key is in use — a claude.ai OAuth login,
	// a bearer token, or a third-party cloud provider. This is what a
	// subscription-authenticated session reports, so it is not an error state.
	APIKeySourceNone = "none"
)

// StopFailureInput contains data for StopFailure hooks.
type StopFailureInput struct {
	BaseHookInput
	Error                AssistantMessageError `json:"error"`
	ErrorDetails         string                `json:"error_details,omitempty"`
	LastAssistantMessage string                `json:"last_assistant_message,omitempty"`
}

// HookType implements HookInput.
func (StopFailureInput) HookType() HookType { return HookTypeStopFailure }

// Base implements HookInput.
func (i StopFailureInput) Base() BaseHookInput { return i.BaseHookInput }

// TaskCompletedInput contains data for TaskCompleted hooks.
type TaskCompletedInput struct {
	BaseHookInput
	TaskID          string `json:"task_id"`
	TaskSubject     string `json:"task_subject"`
	TaskDescription string `json:"task_description,omitempty"`
	TeammateName    string `json:"teammate_name,omitempty"`
	// Deprecated: sessions have a single implicit team; this carries the
	// session-derived team name and will be removed upstream in a future
	// release.
	TeamName string `json:"team_name,omitempty"`
}

// HookType implements HookInput.
func (TaskCompletedInput) HookType() HookType { return HookTypeTaskCompleted }

// Base implements HookInput.
func (i TaskCompletedInput) Base() BaseHookInput { return i.BaseHookInput }

// TaskCreatedInput contains data for TaskCreated hooks.
type TaskCreatedInput struct {
	BaseHookInput
	TaskID          string `json:"task_id"`
	TaskSubject     string `json:"task_subject"`
	TaskDescription string `json:"task_description,omitempty"`
	TeammateName    string `json:"teammate_name,omitempty"`
	// Deprecated: sessions have a single implicit team; this carries the
	// session-derived team name and will be removed upstream in a future
	// release.
	TeamName string `json:"team_name,omitempty"`
}

// HookType implements HookInput.
func (TaskCreatedInput) HookType() HookType { return HookTypeTaskCreated }

// Base implements HookInput.
func (i TaskCreatedInput) Base() BaseHookInput { return i.BaseHookInput }

// TeammateIdleInput contains data for TeammateIdle hooks.
type TeammateIdleInput struct {
	BaseHookInput
	TeammateName string `json:"teammate_name"`
	// Deprecated: sessions have a single implicit team; this carries the
	// session-derived team name and will be removed upstream in a future
	// release.
	TeamName string `json:"team_name"`
}

// HookType implements HookInput.
func (TeammateIdleInput) HookType() HookType { return HookTypeTeammateIdle }

// Base implements HookInput.
func (i TeammateIdleInput) Base() BaseHookInput { return i.BaseHookInput }

// UserPromptExpansionInput contains data for UserPromptExpansion hooks.
type UserPromptExpansionInput struct {
	BaseHookInput
	ExpansionType string `json:"expansion_type"`
	CommandName   string `json:"command_name"`
	CommandArgs   string `json:"command_args"`
	CommandSource string `json:"command_source,omitempty"`
	Prompt        string `json:"prompt"`
}

// HookType implements HookInput.
func (UserPromptExpansionInput) HookType() HookType { return HookTypeUserPromptExpansion }

// Base implements HookInput.
func (i UserPromptExpansionInput) Base() BaseHookInput { return i.BaseHookInput }

// WorktreeCreateInput contains data for WorktreeCreate hooks.
type WorktreeCreateInput struct {
	BaseHookInput
	Name string `json:"name"`
}

// HookType implements HookInput.
func (WorktreeCreateInput) HookType() HookType { return HookTypeWorktreeCreate }

// Base implements HookInput.
func (i WorktreeCreateInput) Base() BaseHookInput { return i.BaseHookInput }

// WorktreeRemoveInput contains data for WorktreeRemove hooks.
type WorktreeRemoveInput struct {
	BaseHookInput
	WorktreePath string `json:"worktree_path"`
}

// HookType implements HookInput.
func (WorktreeRemoveInput) HookType() HookType { return HookTypeWorktreeRemove }

// Base implements HookInput.
func (i WorktreeRemoveInput) Base() BaseHookInput { return i.BaseHookInput }

// HookJSONOutput is the output format for hook callbacks.
// This is what hooks can return to control behavior.
type HookJSONOutput struct {
	Continue           bool                   `json:"continue,omitempty"`
	SuppressOutput     bool                   `json:"suppressOutput,omitempty"`
	StopReason         string                 `json:"stopReason,omitempty"`
	Decision           string                 `json:"decision,omitempty"` // "approve" or "block"
	SystemMessage      string                 `json:"systemMessage,omitempty"`
	Reason             string                 `json:"reason,omitempty"`
	TerminalSequence   string                 `json:"terminalSequence,omitempty"`
	HookSpecificOutput map[string]interface{} `json:"hookSpecificOutput,omitempty"`
}

// PermissionUpdate represents an operation for updating permissions.
// Field usage depends on Type; zero-valued fields are omitted from the wire.
type PermissionUpdate struct {
	Type        PermissionUpdateType        `json:"type"`
	Rules       []PermissionRule            `json:"rules,omitempty"`    // addRules / replaceRules / removeRules
	Behavior    PermissionBehavior          `json:"behavior,omitempty"` // addRules / replaceRules / removeRules
	Destination PermissionUpdateDestination `json:"destination"`
	Mode        PermissionMode              `json:"mode,omitempty"`        // setMode
	Directories []string                    `json:"directories,omitempty"` // addDirectories / removeDirectories
}

// PermissionRule represents a permission rule value.
type PermissionRule struct {
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionUpdateType identifies which PermissionUpdate variant a value represents.
type PermissionUpdateType string

const (
	// PermissionUpdateTypeAddRules appends rules to the destination.
	PermissionUpdateTypeAddRules PermissionUpdateType = "addRules"
	// PermissionUpdateTypeReplaceRules replaces rules in the destination.
	PermissionUpdateTypeReplaceRules PermissionUpdateType = "replaceRules"
	// PermissionUpdateTypeRemoveRules removes rules from the destination.
	PermissionUpdateTypeRemoveRules PermissionUpdateType = "removeRules"
	// PermissionUpdateTypeSetMode updates the permission mode.
	PermissionUpdateTypeSetMode PermissionUpdateType = "setMode"
	// PermissionUpdateTypeAddDirectories appends directories to the destination.
	PermissionUpdateTypeAddDirectories PermissionUpdateType = "addDirectories"
	// PermissionUpdateTypeRemoveDirectories removes directories from the destination.
	PermissionUpdateTypeRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionUpdateDestination indicates where a PermissionUpdate writes its changes.
type PermissionUpdateDestination string

const (
	// PermissionDestinationUserSettings writes to user settings.
	PermissionDestinationUserSettings PermissionUpdateDestination = "userSettings"
	// PermissionDestinationProjectSettings writes to project settings.
	PermissionDestinationProjectSettings PermissionUpdateDestination = "projectSettings"
	// PermissionDestinationLocalSettings writes to local settings.
	PermissionDestinationLocalSettings PermissionUpdateDestination = "localSettings"
	// PermissionDestinationSession writes to the current session.
	PermissionDestinationSession PermissionUpdateDestination = "session"
	// PermissionDestinationCLIArg writes to the CLI argument source.
	PermissionDestinationCLIArg PermissionUpdateDestination = "cliArg"
)

// PermissionBehavior controls permission behavior for rules.
type PermissionBehavior string

const (
	// PermissionBehaviorAllow allows the action.
	PermissionBehaviorAllow PermissionBehavior = "allow"
	// PermissionBehaviorDeny denies the action.
	PermissionBehaviorDeny PermissionBehavior = "deny"
	// PermissionBehaviorAsk prompts the user.
	PermissionBehaviorAsk PermissionBehavior = "ask"
)

// HookResult is the outcome of a hook callback.
//
// For most hooks, set Continue=true to allow execution to proceed.
// For Stop hooks, use Decision/Reason/SystemMessage to control whether
// the session exits or continues with a new prompt (Ralph Wiggum pattern).
//
// For PreToolUse hooks, Modify is automatically translated into the
// hookSpecificOutput.updatedInput format expected by the CLI. For
// PostToolUse hooks, UpdatedToolOutput is automatically translated
// into hookSpecificOutput.updatedToolOutput. Set HookSpecificOutput
// directly for finer control over the response.
type HookResult struct {
	Continue bool                   // Continue execution (false = abort)
	Modify   map[string]interface{} // Modifications to apply

	// WatchPaths registers filesystem paths the CLI should watch and
	// re-fire the hook on changes. Honored by SessionStart, CwdChanged,
	// and FileChanged hooks. Empty slice or nil omits the field on the
	// wire.
	WatchPaths []string

	// Decision controls session exit for Stop hooks.
	// "approve" allows the session to exit normally.
	// "block" prevents exit and reinjects Reason as a new prompt.
	Decision string

	// Reason is the new prompt to inject when Decision="block".
	// This allows Stop hooks to continue the conversation with a new task.
	Reason string

	// SystemMessage is displayed to Claude as context when blocking exit.
	// Use this to provide iteration counts or other status information.
	SystemMessage string

	// TerminalSequence is a terminal escape sequence (e.g. OSC 9 / OSC 777
	// desktop-notification) the CLI will emit verbatim on the SDK consumer's
	// behalf after running the hook. Empty string omits the field on the
	// wire. The CLI restricts the allowlist to notification/title OSCs
	// (0, 1, 2, 9, 99, 777) and BEL; anything else is dropped CLI-side.
	// Honored on any hook return, sync or async.
	TerminalSequence string

	// SuppressOriginalPrompt asks the CLI to omit the original user prompt
	// from the block message it returns when the hook blocks. Honored on
	// UserPromptSubmit (sdk.d.ts v0.3.150 L5808) and UserPromptExpansion
	// (sdk.d.ts v0.3.241 L8219) hooks, with identical semantics on both;
	// silently dropped for other hook types. Nil (the default) leaves the
	// wire field unset; a pointer to false explicitly opts out. Translates
	// into hookSpecificOutput.suppressOriginalPrompt. Useful when the prompt
	// itself was the reason for the block (PII, credentials, etc.).
	SuppressOriginalPrompt *bool

	// ReloadSkills, when set on a SessionStart hook return, asks the CLI to
	// re-scan skill and command directories after SessionStart hooks complete.
	// Honored only on SessionStart hooks and silently dropped for other hook
	// types; nil leaves the wire field unset and a pointer to false explicitly
	// opts out. Translates into hookSpecificOutput.reloadSkills per sdk.d.ts
	// v0.3.168 L3990.
	ReloadSkills *bool `json:"reloadSkills,omitempty"`

	// DisplayContent replaces the displayed delta for MessageDisplay hooks
	// without changing the stored message or model-visible content. Honored
	// only on MessageDisplay hooks and silently dropped for other hook types.
	// Translates into hookSpecificOutput.displayContent per sdk.d.ts v0.3.168
	// L1182-L1191.
	DisplayContent *string

	// AdditionalContext is non-error feedback delivered to the model or
	// subagent; the conversation continues so it can act on the context.
	// Translates into hookSpecificOutput.additionalContext per sdk.d.ts
	// v0.3.168 for the hook events whose envelope accepts the field:
	// Notification (L1251), PostToolBatch (L2107), PostToolUseFailure
	// (L2132), PostToolUse (L2149), PreToolUse (L2175), SessionStart
	// (L3982), Setup (L5695), Stop (L5841-L5845), SubagentStart (L5855),
	// SubagentStop (L5882-L5886), UserPromptExpansion (L6087), and
	// UserPromptSubmit (L6098). Silently dropped for other hook types.
	// Empty string omits the field on the wire. Composes with an explicit
	// HookSpecificOutput map: the typed value overwrites
	// additionalContext but preserves any other keys the caller set.
	AdditionalContext string

	// UpdatedToolOutput replaces the tool output sent to the model on
	// PostToolUse hooks. The value is forwarded verbatim as
	// hookSpecificOutput.updatedToolOutput; any JSON-encodable value is
	// accepted. Ignored for non-PostToolUse hooks. Set HookSpecificOutput
	// directly for finer control or to address the deprecated MCP-only
	// updatedMCPToolOutput field.
	UpdatedToolOutput interface{}

	// ClassifierContext is host-asserted context shown to the auto-mode
	// permission classifier alongside this tool call's result. Translates
	// into hookSpecificOutput.classifierContext on PostToolUse hooks only;
	// silently dropped elsewhere. Empty string omits the field
	// (sdk.d.ts v0.3.241 L2339).
	//
	// In a live session the classifier may weigh a user statement relayed
	// here as user intent — it can satisfy a consent bar a user turn would
	// satisfy, though never a hard boundary. Values restored from saved
	// session state are treated as unverified context only. Relay discipline
	// is the host's obligation: put ONLY genuine user statements in
	// intent-bearing positions, never tool output or model text dressed as
	// one. Content placed here reaches the classifier with host-application
	// framing, so copying untrusted tool output or third-party text into it
	// hands that text the host's authority.
	//
	// Constraints that silently drop the value rather than erroring:
	//
	//   - Capped at 2000 UTF-16 code units, a budget shared across every hook
	//     contributing to one call. Astral characters (emoji and the like)
	//     count as two.
	//   - Honored on synchronous hook responses only. An async hook's late
	//     response arrives after the result message is frozen, and the field
	//     in it is ignored.
	//   - Applies only to calls the classifier transcript shows. Read-only
	//     lookups the transcript omits (file reads, searches), inner REPL
	//     calls and remote-engine shells produce no per-result line, so
	//     context attached to them is unused.
	//
	// It is bound to a single call id and sized for a short assertion — not a
	// delivery channel for relaying messages or events.
	//
	// Rewrite integrity: if the assertion describes output being rewritten,
	// return it in the SAME hook result as the rewrite, so it is dropped
	// automatically if that rewrite is rejected or superseded. An assertion
	// returned without a rewrite is never invalidated by another hook's
	// rewrite, so a non-rewriting hook should assert only what holds
	// regardless. Do NOT return an identity rewrite just to pair an
	// assertion: hooks run in parallel on the ORIGINAL output, so an identity
	// rewrite competes last-write-wins with sibling rewrites and can clobber
	// a real redaction.
	ClassifierContext string

	// PermissionDecision lets a pre-phase hook rule on the action it is
	// gating. Honored on PreToolUse (sdk.d.ts v0.3.251 L2500) and
	// PreModelSwitch (L2487); silently dropped elsewhere. Empty string omits
	// the field on the wire. Composes with an explicit HookSpecificOutput
	// map the same way AdditionalContext does: the typed value overwrites
	// permissionDecision and leaves the caller's other keys alone.
	//
	// Note the value sets differ. PreToolUse takes all four constants;
	// PreModelSwitch takes allow, deny and ask only, and there is no
	// deferred model switch to hand off to.
	PermissionDecision HookPermissionDecision

	// PermissionDecisionReason is the human-readable justification shown
	// alongside PermissionDecision. Honored on the same hook events, and
	// meaningless without one.
	PermissionDecisionReason string

	// HookSpecificOutput provides raw hookSpecificOutput for the CLI
	// response. When set, this takes precedence over auto-translation
	// of Modify. Use this for finer control over permissionDecision,
	// additionalContext, or other hook-specific fields.
	HookSpecificOutput map[string]interface{}
}

// HookPermissionDecision is a pre-phase hook's ruling on the action it gates
// (sdk.d.ts v0.3.251 L878).
type HookPermissionDecision string

const (
	// HookPermissionAllow proceeds without asking the user. On PreModelSwitch
	// this also skips the interactive cache-miss confirm.
	HookPermissionAllow HookPermissionDecision = "allow"

	// HookPermissionDeny cancels the action.
	HookPermissionDeny HookPermissionDecision = "deny"

	// HookPermissionAsk puts the decision to the user. A headless session has
	// nobody to ask and refuses instead.
	HookPermissionAsk HookPermissionDecision = "ask"

	// HookPermissionDefer hands the decision to the normal permission flow as
	// though the hook had not run. PreToolUse only.
	HookPermissionDefer HookPermissionDecision = "defer"
)

// AgentDefinition defines a specialized subagent.
type AgentDefinition struct {
	Name        string `json:"-"` // Agent identifier
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	// Tools is the array of allowed tool names for this agent. When
	// omitted, the agent inherits all tools from its parent.
	//
	// Note: passing "Skill" here is deprecated as of TS SDK v0.3.150
	// (sdk.d.ts L44). Use AgentDefinition.Skills for per-agent skill
	// preload instead.
	Tools                              []string             `json:"tools,omitempty"`
	Model                              string               `json:"model,omitempty"`
	DisallowedTools                    []string             `json:"disallowedTools,omitempty"`
	MCPServers                         []AgentMCPServerSpec `json:"mcpServers,omitempty"`
	CriticalSystemReminderExperimental string               `json:"criticalSystemReminder_EXPERIMENTAL,omitempty"`
	Skills                             []string             `json:"skills,omitempty"`
	InitialPrompt                      string               `json:"initialPrompt,omitempty"`
	MaxTurns                           int                  `json:"maxTurns,omitempty"`
	Background                         *bool                `json:"background,omitempty"`
	Memory                             AgentMemoryScope     `json:"memory,omitempty"`
	Effort                             AgentEffort          `json:"effort,omitempty"`
	PermissionMode                     PermissionMode       `json:"permissionMode,omitempty"`
	// Observer names an agent type auto-spawned as a background observer
	// whenever this agent runs. The observer receives read-only activity
	// digests and reports via the ObserverReport tool; it never
	// participates in the task (sdk.d.ts v0.3.201).
	Observer string `json:"observer,omitempty"`
	// ObserverMessage is a supplemental postamble appended (after the
	// harness-owned default) to each activity digest sent to the observer.
	// Blank values are ignored.
	ObserverMessage string `json:"observerMessage,omitempty"`
}

// MarshalJSON emits the TypeScript SDK agent wire shape.
func (a AgentDefinition) MarshalJSON() ([]byte, error) {
	type agentDefinitionJSON struct {
		Description                        string               `json:"description"`
		Prompt                             string               `json:"prompt"`
		Tools                              []string             `json:"tools,omitempty"`
		Model                              string               `json:"model,omitempty"`
		DisallowedTools                    []string             `json:"disallowedTools,omitempty"`
		MCPServers                         []AgentMCPServerSpec `json:"mcpServers,omitempty"`
		CriticalSystemReminderExperimental string               `json:"criticalSystemReminder_EXPERIMENTAL,omitempty"`
		Skills                             []string             `json:"skills,omitempty"`
		InitialPrompt                      string               `json:"initialPrompt,omitempty"`
		MaxTurns                           int                  `json:"maxTurns,omitempty"`
		Background                         *bool                `json:"background,omitempty"`
		Memory                             AgentMemoryScope     `json:"memory,omitempty"`
		Effort                             *AgentEffort         `json:"effort,omitempty"`
		PermissionMode                     PermissionMode       `json:"permissionMode,omitempty"`
		Observer                           string               `json:"observer,omitempty"`
		ObserverMessage                    string               `json:"observerMessage,omitempty"`
	}

	out := agentDefinitionJSON{
		Description:                        a.Description,
		Prompt:                             a.Prompt,
		Tools:                              a.Tools,
		Model:                              a.Model,
		DisallowedTools:                    a.DisallowedTools,
		MCPServers:                         a.MCPServers,
		CriticalSystemReminderExperimental: a.CriticalSystemReminderExperimental,
		Skills:                             a.Skills,
		InitialPrompt:                      a.InitialPrompt,
		MaxTurns:                           a.MaxTurns,
		Background:                         a.Background,
		Memory:                             a.Memory,
		PermissionMode:                     a.PermissionMode,
		Observer:                           a.Observer,
		ObserverMessage:                    a.ObserverMessage,
	}
	if !a.Effort.IsZero() {
		out.Effort = &a.Effort
	}

	return json.Marshal(out)
}

// AgentMemoryScope controls which memory scope is available to an agent.
type AgentMemoryScope string

const (
	// AgentMemoryUser enables user memory for the agent.
	AgentMemoryUser AgentMemoryScope = "user"
	// AgentMemoryProject enables project memory for the agent.
	AgentMemoryProject AgentMemoryScope = "project"
	// AgentMemoryLocal enables local memory for the agent.
	AgentMemoryLocal AgentMemoryScope = "local"
)

// AgentEffort is the AgentDefinition effort union: EffortLevel or numeric budget.
type AgentEffort struct {
	Level   EffortLevel
	Numeric *int
}

// IsZero reports whether no effort was configured.
func (e AgentEffort) IsZero() bool {
	return e.Level == "" && e.Numeric == nil
}

// MarshalJSON emits either the numeric or string effort variant.
func (e AgentEffort) MarshalJSON() ([]byte, error) {
	if e.Numeric != nil {
		return json.Marshal(*e.Numeric)
	}
	if e.Level != "" {
		return json.Marshal(e.Level)
	}
	return []byte("null"), nil
}

// UnmarshalJSON decodes either the numeric or string effort variant.
func (e *AgentEffort) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*e = AgentEffort{}
		return nil
	}

	var level EffortLevel
	if err := json.Unmarshal(data, &level); err == nil {
		*e = AgentEffort{Level: level}
		return nil
	}

	var numeric int
	if err := json.Unmarshal(data, &numeric); err != nil {
		return err
	}
	*e = AgentEffort{Numeric: &numeric}
	return nil
}

// AgentMCPServerSpec references a top-level MCP server or defines inline servers.
type AgentMCPServerSpec struct {
	// Name references a server defined in Options.MCPServers by key. Mutually exclusive with Inline.
	Name string
	// Inline defines servers locally for this agent. Mutually exclusive with Name.
	Inline map[string]MCPServerConfig
}

// MarshalJSON emits the AgentMCPServerSpec discriminated union.
func (s AgentMCPServerSpec) MarshalJSON() ([]byte, error) {
	if s.Name != "" {
		return json.Marshal(s.Name)
	}
	if s.Inline != nil {
		return json.Marshal(s.Inline)
	}
	return []byte("null"), nil
}

// UnmarshalJSON decodes a named or inline AgentMCPServerSpec.
func (s *AgentMCPServerSpec) UnmarshalJSON(data []byte) error {
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// json.RawMessage preserves leading whitespace, so skip it before
	// dispatching on the first significant byte.
	for len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t' || raw[0] == '\n' || raw[0] == '\r') {
		raw = raw[1:]
	}
	if len(raw) == 0 || string(raw) == "null" {
		*s = AgentMCPServerSpec{}
		return nil
	}

	switch raw[0] {
	case '"':
		var name string
		if err := json.Unmarshal(raw, &name); err != nil {
			return err
		}
		*s = AgentMCPServerSpec{Name: name}
	case '{':
		var inline map[string]MCPServerConfig
		if err := json.Unmarshal(raw, &inline); err != nil {
			return err
		}
		*s = AgentMCPServerSpec{Inline: inline}
	default:
		return fmt.Errorf("agent MCP server spec must be string or object")
	}
	return nil
}

// SessionOptions configures session behavior.
type SessionOptions struct {
	SessionID       string // Explicit session ID (empty = auto-generate)
	Resume          string // Session ID to resume
	ForkFrom        string // Session ID to fork from
	ForkSession     bool   // Fork to a new session ID when resuming
	ResumeSessionAt string // Resume session at a specific message UUID

	// ResumeDropsTurn declares, for a truncating ResumeSessionAt resume, the
	// prompt UUID of the turn the resume intends to discard. The CLI checks
	// at fork time that every entry past the fork point is attributable to
	// that turn and refuses the resume when the discarded range holds
	// anything else — a queued user message or task notification the session
	// absorbed mid-turn that the caller had not observed. The refusal
	// arrives as an error_during_execution result whose message starts with
	// "Resume rejected by --resume-drops-turn:"; it is deterministic, so
	// consumers must route it to their rewind-recovery path rather than
	// retry the same fork request.
	//
	// Fork at the kept turn's LAST chain entry, whatever it is —
	// ResumeSessionAt accepts any chain UUID, not just an assistant one.
	// Consumed only by the headless boot path the Go SDK drives; interactive
	// `claude --resume` ignores both fields. Empty leaves the unvalidated
	// truncation behavior in place.
	ResumeDropsTurn string
}

// MCPServerConfig configures an MCP server.
type MCPServerConfig struct {
	Type    string                `json:"type,omitempty"`    // "stdio", "sse", "http", or legacy "socket"
	Command string                `json:"command,omitempty"` // Command to start server (for stdio)
	Args    []string              `json:"args,omitempty"`    // Command arguments
	Env     map[string]string     `json:"env,omitempty"`     // Environment variables
	URL     string                `json:"url,omitempty"`     // Remote server URL (for sse/http)
	Headers map[string]string     `json:"headers,omitempty"` // Remote server headers (for sse/http)
	Tools   []MCPServerToolPolicy `json:"tools,omitempty"`   // Remote server tool permission policies
	Address string                `json:"address,omitempty"` // Socket address (for socket type)
	// Timeout is the per-server tool-call timeout in milliseconds. Overrides
	// the MCP_TOOL_TIMEOUT environment variable for this server. Hard wall-
	// clock limit per call; progress notifications do not extend it.
	// Values below 1000ms are ignored and fall through to MCP_TOOL_TIMEOUT
	// or the default.
	Timeout *int `json:"timeout,omitempty"`
	// AlwaysLoad, when true, forces every tool from this server to be included
	// in the prompt instead of deferred behind tool search. Equivalent to
	// setting defer_loading: false on the API. Default: tools are deferred
	// when tool search is enabled. Side effect: setting this blocks startup
	// until the server is connected (capped at the standard 5s connect
	// timeout), even though MCP startup is otherwise non-blocking; the tools
	// must be present when the turn-1 prompt is built.
	AlwaysLoad *bool `json:"alwaysLoad,omitempty"`
}

// MCPServerToolPolicy configures a per-tool permission policy for remote MCP servers.
type MCPServerToolPolicy struct {
	Name             string `json:"name"`
	PermissionPolicy string `json:"permission_policy"`
	// OrgMaxPermission is the org admin's per-tool ceiling. It drives the
	// auto-mode isOrgAskCeiling gate so an admin 'ask' cap forces a user
	// prompt even in auto mode. One of MCPOrgMaxPermission*; empty when unset.
	OrgMaxPermission string `json:"org_max_permission,omitempty"`
}

const (
	MCPToolPolicyAllowAlways = "always_allow"
	MCPToolPolicyAskAlways   = "always_ask"
	MCPToolPolicyDenyAlways  = "always_deny"
)

const (
	MCPOrgMaxPermissionAllow   = "allow"
	MCPOrgMaxPermissionAsk     = "ask"
	MCPOrgMaxPermissionBlocked = "blocked"
)

// SkillsConfig controls how Skills are loaded.
type SkillsConfig struct {
	// EnableSkills enables Skills loading from filesystem.
	// Default: true
	EnableSkills bool

	// UserSkillsDir overrides default ~/.claude/skills/ path.
	// Empty string uses default.
	UserSkillsDir string

	// ProjectSkillsDir overrides default ./.claude/skills/ path.
	// Empty string uses default.
	ProjectSkillsDir string

	// SettingSources controls which Skills locations to load.
	// Options: "user", "project"
	// Default: ["user", "project"]
	SettingSources []string
}

// WithSkills enables Skills with custom configuration.
//
// Example:
//
//	WithSkills(SkillsConfig{
//	    EnableSkills:     true,
//	    ProjectSkillsDir: "./custom-skills",
//	    SettingSources:   []string{"project"},
//	})
func WithSkills(config SkillsConfig) Option {
	return func(o *Options) {
		o.SkillsConfig = config
	}
}

// WithSkillsDisabled disables Skills loading.
func WithSkillsDisabled() Option {
	return func(o *Options) {
		o.SkillsConfig.EnableSkills = false
	}
}

// WithSystemPromptPreset sets a preset system prompt configuration.
// Use "claude_code" to get Claude Code's default system prompt.
func WithSystemPromptPreset(preset string, append string) Option {
	return func(o *Options) {
		o.SystemPromptPreset = &SystemPromptConfig{
			Type:   "preset",
			Preset: preset,
			Append: append,
		}
	}
}

// WithFallbackModel sets the fallback model list used when the primary
// model is overloaded or unavailable. Accepts a comma-separated list to try
// in order; the primary model is re-tried at the start of each user turn.
func WithFallbackModel(model string) Option {
	return func(o *Options) {
		o.FallbackModel = model
	}
}

// WithCwd sets the current working directory for the agent.
func WithCwd(cwd string) Option {
	return func(o *Options) {
		o.Cwd = cwd
	}
}

// WithAdditionalDirectories sets additional directories Claude can access.
func WithAdditionalDirectories(dirs []string) Option {
	return func(o *Options) {
		o.AdditionalDirectories = dirs
	}
}

// WithAllowDangerouslySkipPermissions enables bypassing permissions.
// Required when using PermissionModeBypassAll.
func WithAllowDangerouslySkipPermissions(allow bool) Option {
	return func(o *Options) {
		o.AllowDangerouslySkipPermissions = allow
	}
}

// WithSettingSources controls which filesystem settings to load.
// Options: SettingSourceUser, SettingSourceProject, SettingSourceLocal.
func WithSettingSources(sources []SettingSource) Option {
	return func(o *Options) {
		o.SettingSources = sources
	}
}

// WithSettingsPath loads explicit settings from a JSON file path.
func WithSettingsPath(path string) Option {
	return func(o *Options) {
		o.SettingsPath = path
		o.Settings = nil
	}
}

// WithSettings supplies inline Claude Code settings.
func WithSettings(settings Settings) Option {
	return func(o *Options) {
		o.SettingsPath = ""
		o.Settings = &settings
	}
}

// WithManagedSettings supplies inline managed settings.
func WithManagedSettings(settings Settings) Option {
	return func(o *Options) {
		o.ManagedSettings = &settings
	}
}

// WithSandbox configures sandbox behavior programmatically.
func WithSandbox(sandbox *SandboxSettings) Option {
	return func(o *Options) {
		o.Sandbox = sandbox
	}
}

// WithBetas enables beta features.
//
// Each value is an API beta header name. They are joined and passed to the
// CLI as --betas a,b,c.
//
// Example:
//
//	WithBetas([]string{"context-1m-2025-08-07"})
func WithBetas(betas []string) Option {
	return func(o *Options) {
		o.Betas = betas
	}
}

// WithDebug enables debug logging from the CLI.
func WithDebug(debug bool) Option {
	return func(o *Options) {
		o.Debug = debug
	}
}

// WithDebugFile writes debug logs to the specified file.
func WithDebugFile(path string) Option {
	return func(o *Options) {
		o.DebugFile = path
	}
}

// WithExcludeDynamicSystemPromptSections moves per-machine sections (cwd,
// env info, memory paths, git status) out of the system prompt and into the
// first user message.
//
// Enable this when cross-invocation prompt-cache reuse matters more than
// maximally authoritative environment context in the system prompt. The CLI
// only honors this flag with the default system prompt; it is ignored if
// WithSystemPrompt is used to set a custom string.
func WithExcludeDynamicSystemPromptSections(enable bool) Option {
	return func(o *Options) {
		o.ExcludeDynamicSystemPromptSections = enable
	}
}

// WithPlugins loads custom plugins from local paths.
func WithPlugins(plugins []PluginConfig) Option {
	return func(o *Options) {
		o.Plugins = plugins
	}
}

// WithOutputFormat defines structured output format for agent results.
func WithOutputFormat(format *OutputFormat) Option {
	return func(o *Options) {
		o.OutputFormat = format
	}
}

// WithAllowedTools sets the list of allowed tool names.
// If empty, all tools are allowed.
func WithAllowedTools(tools []string) Option {
	return func(o *Options) {
		o.AllowedTools = tools
	}
}

// WithDisallowedTools sets the list of disallowed tool names.
func WithDisallowedTools(tools []string) Option {
	return func(o *Options) {
		o.DisallowedTools = tools
	}
}

// WithTools configures available tools using preset or explicit list.
func WithTools(config *ToolsConfig) Option {
	return func(o *Options) {
		o.Tools = config
	}
}

// WithThinking controls Claude's thinking/reasoning behavior.
func WithThinking(thinking *ThinkingConfig) Option {
	return func(o *Options) {
		o.Thinking = thinking
	}
}

// WithEffort controls how much effort Claude puts into its response.
func WithEffort(effort EffortLevel) Option {
	return func(o *Options) {
		o.Effort = effort
	}
}

// WithMaxBudgetUsd sets the maximum budget in USD for the query.
func WithMaxBudgetUsd(budget float64) Option {
	return func(o *Options) {
		o.MaxBudgetUsd = &budget
	}
}

// WithTaskBudget sets the maximum task budget for the query.
func WithTaskBudget(total int) Option {
	return func(o *Options) {
		o.TaskBudget = &TaskBudget{Total: total}
	}
}

// WithMaxThinkingTokens sets the maximum tokens for thinking process.
//
// Deprecated: Use WithThinking instead.
func WithMaxThinkingTokens(tokens int) Option {
	return func(o *Options) {
		o.MaxThinkingTokens = &tokens
	}
}

// WithMaxTurns sets the maximum conversation turns.
func WithMaxTurns(turns int) Option {
	return func(o *Options) {
		o.MaxTurns = &turns
	}
}

// WithEnableFileCheckpointing enables file change tracking for rewinding.
func WithEnableFileCheckpointing(enable bool) Option {
	return func(o *Options) {
		o.EnableFileCheckpointing = enable
	}
}

// WithIncludePartialMessages includes partial message events in stream.
func WithIncludePartialMessages(include bool) Option {
	return func(o *Options) {
		o.IncludePartialMessages = include
	}
}

// WithContinue continues the most recent conversation.
func WithContinue(cont bool) Option {
	return func(o *Options) {
		o.Continue = cont
	}
}

// WithStderr sets a callback for stderr output from the CLI.
func WithStderr(callback func(data string)) Option {
	return func(o *Options) {
		o.Stderr = callback
	}
}

// WithNoSessionPersistence disables session persistence.
// Sessions will not be saved to disk and cannot be resumed.
// Useful for testing to avoid polluting session history.
func WithNoSessionPersistence() Option {
	return func(o *Options) {
		o.NoSessionPersistence = true
	}
}

// WithConfigDir sets a custom config directory for full isolation.
// This overrides the default ~/.claude directory, isolating the CLI from
// user settings, hooks, sessions, and other configuration.
// The CLAUDE_CONFIG_DIR environment variable is set to this value.
// Useful for testing to create a completely sandboxed environment.
func WithConfigDir(dir string) Option {
	return func(o *Options) {
		o.ConfigDir = dir
	}
}

// WithStrictMCPConfig only uses MCP servers from MCPServers config.
// When enabled, MCP configurations from settings files are ignored.
// Useful for testing to ensure only test MCP servers are used.
func WithStrictMCPConfig(strict bool) Option {
	return func(o *Options) {
		o.StrictMCPConfig = strict
	}
}

// WithTaskListID sets the shared task list ID.
//
// Multiple Claude instances with the same ID share the same task list.
// Tasks persist at ~/.claude/tasks/{id}/. The CLAUDE_CODE_TASK_LIST_ID
// environment variable is automatically set for the CLI subprocess.
//
// Example:
//
//	client, _ := claudeagent.NewClient(
//	    claudeagent.WithTaskListID("my-project"),
//	)
func WithTaskListID(id string) Option {
	return func(o *Options) {
		o.TaskListID = id
		if o.Env == nil {
			o.Env = make(map[string]string)
		}
		o.Env["CLAUDE_CODE_TASK_LIST_ID"] = id
	}
}

// WithTaskStore sets a custom task storage backend.
//
// Use this to provide alternative storage implementations such as:
//   - MemoryTaskStore for testing
//   - PostgresTaskStore for distributed coordination
//   - RedisTaskStore for real-time updates
//
// When using a custom store, the SDK accesses tasks through this store
// while the CLI continues using its default file-based storage. For full
// synchronization, consider implementing an MCP proxy pattern.
//
// Example:
//
//	store := claudeagent.NewMemoryTaskStore()
//	client, _ := claudeagent.NewClient(
//	    claudeagent.WithTaskStore(store),
//	)
func WithTaskStore(store TaskStore) Option {
	return func(o *Options) {
		o.TaskStore = store
	}
}

// DefaultOptions returns options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		Model:          "claude-sonnet-4-5-20250929",
		PermissionMode: PermissionModeDefault,
		Env:            make(map[string]string),
		Hooks:          make(map[HookType][]HookConfig),
		Agents:         make(map[string]AgentDefinition),
		MCPServers:     make(map[string]MCPServerConfig),
		SkillsConfig: SkillsConfig{
			EnableSkills:   true,
			SettingSources: []string{"user", "project"},
		},
		Verbose: false,
	}
}
