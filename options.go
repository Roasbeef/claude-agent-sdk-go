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

	// FallbackModel is the model to use if primary fails.
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

	// Environment variables to pass to the CLI subprocess.
	// ANTHROPIC_API_KEY should be set here or in the parent environment.
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
	AllowedTools []string

	// DisallowedTools is a list of disallowed tool names.
	DisallowedTools []string

	// Tools configures available tools.
	// Can be a list of tool names or use preset "claude_code".
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

	// StrictMCPConfig when true, only uses MCP servers from MCPServers config,
	// ignoring all other MCP configurations from settings files.
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
	Type string
	// Path is the absolute or relative path to the plugin directory.
	Path string
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
	// EffortXHigh applies deeper reasoning than high.
	EffortXHigh EffortLevel = "xhigh"
	// EffortMax applies maximum effort.
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

// WithCLIPath sets the path to the Claude Code CLI executable.
//
// If not specified, the CLI will be discovered from the system PATH.
func WithCLIPath(path string) Option {
	return func(o *Options) {
		o.CLIPath = path
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

// ToolPermissionRequest contains details about a tool execution request.
type ToolPermissionRequest struct {
	ToolName  string          // Tool identifier (e.g., "mcp__tickertape__fetch_quote")
	Arguments json.RawMessage // Tool arguments as JSON
	Context   PermissionContext
}

// PermissionContext provides additional context for permission decisions.
type PermissionContext struct {
	SessionID string
	ToolUseID string
	AgentID   string
	Metadata  map[string]interface{}
}

// PermissionResult is the outcome of a permission check.
type PermissionResult interface {
	IsAllow() bool
}

// PermissionAllow indicates permission granted.
type PermissionAllow struct{}

// IsAllow implements PermissionResult.
func (PermissionAllow) IsAllow() bool { return true }

// PermissionDeny indicates permission denied.
type PermissionDeny struct {
	Reason string
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

	// HookTypePostToolBatch fires after a batch of tool calls completes.
	HookTypePostToolBatch HookType = "PostToolBatch"

	// HookTypePermissionRequest fires when permission check requested.
	HookTypePermissionRequest HookType = "PermissionRequest"

	// HookTypePermissionDenied fires when permission is denied.
	HookTypePermissionDenied HookType = "PermissionDenied"

	// HookTypeCwdChanged fires when the current working directory changes.
	HookTypeCwdChanged HookType = "CwdChanged"

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

// BaseHookInput contains common fields for all hook inputs.
type BaseHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	AgentType      string `json:"agent_type,omitempty"`
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
}

// HookType implements HookInput.
func (UserPromptSubmitInput) HookType() HookType { return HookTypeUserPromptSubmit }

// Base implements HookInput.
func (i UserPromptSubmitInput) Base() BaseHookInput { return i.BaseHookInput }

// StopInput contains data for Stop hooks.
type StopInput struct {
	BaseHookInput
	StopHookActive       bool   `json:"stop_hook_active"`
	LastAssistantMessage string `json:"last_assistant_message,omitempty"`
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
	AgentName            string `json:"agent_name"`
	Status               string `json:"status"`
	Result               string `json:"result"`
	StopHookActive       bool   `json:"stop_hook_active"`
	AgentTranscriptPath  string `json:"agent_transcript_path,omitempty"`
	LastAssistantMessage string `json:"last_assistant_message,omitempty"`
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
	Source string `json:"source"` // "startup", "resume", "clear", or "compact"
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
	AssistantMessageErrorBillingError         AssistantMessageError = "billing_error"
	AssistantMessageErrorRateLimit            AssistantMessageError = "rate_limit"
	AssistantMessageErrorInvalidRequest       AssistantMessageError = "invalid_request"
	AssistantMessageErrorServerError          AssistantMessageError = "server_error"
	AssistantMessageErrorUnknown              AssistantMessageError = "unknown"
	AssistantMessageErrorMaxOutputTokens      AssistantMessageError = "max_output_tokens"
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
	TeamName        string `json:"team_name,omitempty"`
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
	TeamName        string `json:"team_name,omitempty"`
}

// HookType implements HookInput.
func (TaskCreatedInput) HookType() HookType { return HookTypeTaskCreated }

// Base implements HookInput.
func (i TaskCreatedInput) Base() BaseHookInput { return i.BaseHookInput }

// TeammateIdleInput contains data for TeammateIdle hooks.
type TeammateIdleInput struct {
	BaseHookInput
	TeammateName string `json:"teammate_name"`
	TeamName     string `json:"team_name"`
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
	HookSpecificOutput map[string]interface{} `json:"hookSpecificOutput,omitempty"`
}

// PermissionUpdate represents an operation for updating permissions.
type PermissionUpdate struct {
	Type        string             // "addRules", "replaceRules", "removeRules", "setMode", "addDirectories", "removeDirectories"
	Rules       []PermissionRule   // For rule operations
	Behavior    PermissionBehavior // "allow", "deny", "ask"
	Destination string             // "userSettings", "projectSettings", "localSettings", "session"
	Mode        PermissionMode     // For setMode
	Directories []string           // For directory operations
}

// PermissionRule represents a permission rule value.
type PermissionRule struct {
	ToolName    string
	RuleContent string
}

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
// hookSpecificOutput.updatedInput format expected by the CLI. Set
// HookSpecificOutput directly for finer control over the response.
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

	// HookSpecificOutput provides raw hookSpecificOutput for the CLI
	// response. When set, this takes precedence over auto-translation
	// of Modify. Use this for finer control over permissionDecision,
	// additionalContext, or other hook-specific fields.
	HookSpecificOutput map[string]interface{}
}

// AgentDefinition defines a specialized subagent.
type AgentDefinition struct {
	Name                               string               `json:"-"` // Agent identifier
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
	Effort                             AgentEffort          `json:"effort,omitempty"`
	PermissionMode                     PermissionMode       `json:"permissionMode,omitempty"`
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
}

// MCPServerToolPolicy configures a per-tool permission policy for remote MCP servers.
type MCPServerToolPolicy struct {
	Name             string `json:"name"`
	PermissionPolicy string `json:"permission_policy"`
}

const (
	MCPToolPolicyAllowAlways = "always_allow"
	MCPToolPolicyAskAlways   = "always_ask"
	MCPToolPolicyDenyAlways  = "always_deny"
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

// WithFallbackModel sets the model to use if primary fails.
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
