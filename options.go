package claudeagent

import (
	"context"
	"encoding/json"
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

	// FallbackModel is the model to use if primary fails.
	FallbackModel string

	// CLIPath is the path to the Claude Code CLI executable.
	// If empty, the CLI will be discovered from PATH.
	CLIPath string

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
	// Example: []string{"context-1m-2025-08-07"}
	Betas []string

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

	// MaxBudgetUsd is the maximum budget in USD for the query.
	MaxBudgetUsd *float64

	// MaxThinkingTokens is the maximum tokens for thinking process.
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

// ToolsConfig configures available tools.
type ToolsConfig struct {
	// Type is "preset" for preset configuration.
	Type string
	// Preset is the preset name (e.g., "claude_code").
	Preset string
	// Tools is a list of specific tool names.
	Tools []string
}

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

// WithCLIPath sets the path to the Claude Code CLI executable.
//
// If not specified, the CLI will be discovered from the system PATH.
func WithCLIPath(path string) Option {
	return func(o *Options) {
		o.CLIPath = path
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

	// HookTypePermissionRequest fires when permission check requested.
	HookTypePermissionRequest HookType = "PermissionRequest"
)

// HookConfig defines a lifecycle callback.
type HookConfig struct {
	Type     HookType     // Hook event type
	Matcher  string       // Glob pattern for tool names (e.g., "*", "fetch_*")
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
}

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
}

// HookType implements HookInput.
func (StopInput) HookType() HookType { return HookTypeStop }

// Base implements HookInput.
func (i StopInput) Base() BaseHookInput { return i.BaseHookInput }

// SubagentStopInput contains data for SubagentStop hooks.
type SubagentStopInput struct {
	BaseHookInput
	AgentName string `json:"agent_name"`
	Status    string `json:"status"`
	Result    string `json:"result"`
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
type HookResult struct {
	Continue bool                   // Continue execution (false = abort)
	Modify   map[string]interface{} // Modifications to apply
}

// AgentDefinition defines a specialized subagent.
type AgentDefinition struct {
	Name        string   // Agent identifier
	Description string   // When to invoke this agent
	Prompt      string   // System instructions for the subagent
	Tools       []string // Tool whitelist (nil = inherit all)
	Model       string   // Optional model override
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
	Type    string            // "stdio" or "socket"
	Command string            // Command to start server (for stdio)
	Args    []string          // Command arguments
	Env     map[string]string // Environment variables
	Address string            // Socket address (for socket type)
}

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
func WithBetas(betas []string) Option {
	return func(o *Options) {
		o.Betas = betas
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

// WithMaxBudgetUsd sets the maximum budget in USD for the query.
func WithMaxBudgetUsd(budget float64) Option {
	return func(o *Options) {
		o.MaxBudgetUsd = &budget
	}
}

// WithMaxThinkingTokens sets the maximum tokens for thinking process.
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
