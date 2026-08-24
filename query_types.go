package claudeagent

// SlashCommand represents an available slash command.
type SlashCommand struct {
	Name         string   `json:"name"`              // Command name (without slash)
	Description  string   `json:"description"`       // Command description
	ArgumentHint string   `json:"argumentHint"`      // Hint for command arguments
	Aliases      []string `json:"aliases,omitempty"` // Alternate names resolving to this command (e.g. /cost and /stats both resolve to /usage)
}

// ModelInfo contains information about an available model.
type ModelInfo struct {
	Value       string `json:"value"`       // Model ID to use in API calls
	DisplayName string `json:"displayName"` // Human-readable model name
	Description string `json:"description"` // Model capabilities description
	// ResolvedModel is the canonical wire model id this row's Value
	// resolves to (e.g. "sonnet" -> "claude-sonnet-5"). Lets hosts match a
	// persisted explicit id against the alias row that covers it. Present
	// only for alias rows (sdk.d.ts v0.3.201).
	ResolvedModel string `json:"resolvedModel,omitempty"`
	// CanonicalModel is the canonical model id used for the pricing lookup
	// (e.g. "claude-opus-4-7"). May differ from the raw Value this row is
	// keyed by (provider-specific ids, aliases). sdk.d.ts v0.3.220 L1274.
	CanonicalModel string `json:"canonicalModel,omitempty"`
	// Provider is the API provider that served this model: one of
	// "firstParty", "bedrock", "vertex", "foundry", "anthropicAws",
	// "anthropicGoogleCloud", "mantle", "gateway". sdk.d.ts v0.3.220 L1278.
	Provider string `json:"provider,omitempty"`
}

// AgentInfo describes a subagent available to the Task tool.
type AgentInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Model       string `json:"model,omitempty"`
}

// SDKControlInitializeResponse is the parsed initialize control response.
type SDKControlInitializeResponse struct {
	Commands              []SlashCommand `json:"commands"`
	Agents                []AgentInfo    `json:"agents"`
	OutputStyle           string         `json:"output_style"`
	AvailableOutputStyles []string       `json:"available_output_styles"`
	Models                []ModelInfo    `json:"models"`
	Account               AccountInfo    `json:"account"`
	FastModeState         string         `json:"fast_mode_state,omitempty"`
	// FastModeDisabledReason explains why fast mode could not serve, when
	// FastModeState is not "on". Absent when nothing blocks it.
	FastModeDisabledReason FastModeDisabledReason `json:"fast_mode_disabled_reason,omitempty"`
	// HooksApplied reports whether the hooks this initialize carried were
	// registered: true on a session's first initialize, and on a repeated
	// initialize from the process that owns the CLI's stdin — its set replaces
	// the one registered earlier. False when a repeated initialize's hooks were
	// ignored, which happens to a client joining a remote session another
	// client configured.
	//
	// Nil means either the request carried no hooks, or the CLI predates the
	// field — and those are not equivalent, because a CLI that predates it
	// ignored hooks on every repeated initialize. So a nil here after a
	// Reinitialize that did carry hooks is not evidence they took effect.
	//
	// Reinitialize is where a false is actionable: a caller reattaching to a
	// session it did not configure keeps whatever hooks the owning client
	// registered, and its own callbacks will never fire (sdk.d.ts v0.3.241
	// L3752).
	HooksApplied *bool `json:"hooks_applied,omitempty"`
}

// McpServerStatus reports the connection status of an MCP server.
type McpServerStatus struct {
	Name       string         `json:"name"`       // Server name
	Status     McpServerState `json:"status"`     // Connection state
	ServerInfo *McpServerInfo `json:"serverInfo"` // Server metadata (if connected)
}

// McpServerState represents MCP server connection states.
type McpServerState string

const (
	// McpServerStateConnected indicates successful connection.
	McpServerStateConnected McpServerState = "connected"
	// McpServerStateFailed indicates connection failure.
	McpServerStateFailed McpServerState = "failed"
	// McpServerStateNeedsAuth indicates authentication required.
	McpServerStateNeedsAuth McpServerState = "needs-auth"
	// McpServerStatePending indicates connection in progress.
	McpServerStatePending McpServerState = "pending"
)

// McpServerInfo contains metadata about a connected MCP server.
type McpServerInfo struct {
	Name    string `json:"name"`    // Server name
	Version string `json:"version"` // Server version
}

// McpSetServersResult is the response from Stream.SetMcpServers.
type McpSetServersResult struct {
	Added   []string          `json:"added"`
	Removed []string          `json:"removed"`
	Errors  map[string]string `json:"errors"`
}

// RewindFilesOptions controls a file checkpoint rewind.
type RewindFilesOptions struct {
	DryRun bool `json:"dryRun,omitempty"`
}

// RewindFilesResult is the response from Stream.RewindFiles.
type RewindFilesResult struct {
	CanRewind    bool     `json:"canRewind"`
	Error        string   `json:"error,omitempty"`
	FilesChanged []string `json:"filesChanged,omitempty"`
	Insertions   int      `json:"insertions,omitempty"`
	Deletions    int      `json:"deletions,omitempty"`
	// SkippedLinks counts tracked files NOT restored or deleted because a
	// symlink, hard link, or other non-regular file was detected at the tracked
	// path (or its parent no longer resolves where it pointed at checkpoint
	// time, or its backup could not be safely read). Populated only by a real
	// (non-dryRun) rewind; absent or 0 means no link-safety refusals occurred.
	// sdk.d.ts v0.3.220 L2699.
	SkippedLinks int `json:"skippedLinks,omitempty"`
}

// ReadFileOptions controls Stream.ReadFile.
type ReadFileOptions struct {
	MaxBytes int `json:"maxBytes,omitempty"`
	// Encoding selects how the CLI encodes file bytes in the response.
	// Valid values are "utf-8" (default; lossy for binary) and "base64"
	// (required for arbitrary binary). Empty string lets the CLI pick its
	// default.
	Encoding string `json:"encoding,omitempty"`
}

// SDKControlReadFileResponse contains file contents from Stream.ReadFile.
type SDKControlReadFileResponse struct {
	Contents  string `json:"contents"`
	AbsPath   string `json:"absPath"`
	Truncated bool   `json:"truncated,omitempty"`
	// Encoding is set to "base64" when the CLI honored a base64 request.
	// Empty string means utf-8, including when older CLIs ignore the request.
	Encoding string `json:"encoding,omitempty"`
}

// SDKControlReloadPluginsResponse reports refreshed session components.
type SDKControlReloadPluginsResponse struct {
	Commands   []SlashCommand    `json:"commands"`
	Agents     []AgentInfo       `json:"agents"`
	Plugins    []PluginInfo      `json:"plugins"`
	McpServers []McpServerStatus `json:"mcpServers"`
	ErrorCount int               `json:"error_count"`
}

// SDKControlReloadSkillsResponse reports refreshed skill commands.
type SDKControlReloadSkillsResponse struct {
	Skills []SlashCommand `json:"skills"`
}

// PluginInfo describes a plugin loaded by the CLI.
type PluginInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Version is the plugin's version as declared in its plugin.json
	// manifest, emitted verbatim. Plugin-author-controlled — validate before
	// trusting. Omitted when the manifest declares no version (sdk.d.ts
	// v0.3.215).
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`
}

// APIProvider names the active CLI API backend. Anthropic OAuth login only
// applies for APIProviderFirstParty; for 3P providers the other AccountInfo
// fields are absent and auth is external (AWS creds, gcloud ADC, etc.).
// APIProviderGateway means the CLI is authenticated against an enterprise
// gateway.
type APIProvider string

const (
	APIProviderFirstParty   APIProvider = "firstParty"
	APIProviderBedrock      APIProvider = "bedrock"
	APIProviderVertex       APIProvider = "vertex"
	APIProviderFoundry      APIProvider = "foundry"
	APIProviderAnthropicAWS APIProvider = "anthropicAws"
	// APIProviderAnthropicGoogleCloud is the Anthropic-managed Google Cloud
	// backend (sdk.d.ts v0.3.215).
	APIProviderAnthropicGoogleCloud APIProvider = "anthropicGoogleCloud"
	APIProviderMantle               APIProvider = "mantle"
	APIProviderGateway              APIProvider = "gateway"
)

// AccountInfo contains user account information.
type AccountInfo struct {
	Email            string      `json:"email,omitempty"`            // User email
	Organization     string      `json:"organization,omitempty"`     // Organization name
	SubscriptionType string      `json:"subscriptionType,omitempty"` // Subscription tier
	TokenSource      string      `json:"tokenSource,omitempty"`      // How token was obtained
	APIKeySource     string      `json:"apiKeySource,omitempty"`     // API key source
	APIProvider      APIProvider `json:"apiProvider,omitempty"`      // Active API backend
}

// SDKControlGetContextUsageResponse mirrors the context usage control response.
type SDKControlGetContextUsageResponse struct {
	Categories           []ContextUsageCategory        `json:"categories"`
	TotalTokens          int                           `json:"totalTokens"`
	MaxTokens            int                           `json:"maxTokens"`
	RawMaxTokens         int                           `json:"rawMaxTokens"`
	Percentage           float64                       `json:"percentage"`
	GridRows             [][]ContextUsageGridCell      `json:"gridRows"`
	Model                string                        `json:"model"`
	MemoryFiles          []ContextUsageMemoryFile      `json:"memoryFiles"`
	McpTools             []ContextUsageMcpTool         `json:"mcpTools"`
	DeferredBuiltinTools []ContextUsageBuiltinTool     `json:"deferredBuiltinTools,omitempty"`
	SystemTools          []ContextUsageSystemTool      `json:"systemTools,omitempty"`
	SystemPromptSections []ContextUsageSection         `json:"systemPromptSections,omitempty"`
	Agents               []ContextUsageAgent           `json:"agents"`
	SlashCommands        *ContextUsageSlashCommands    `json:"slashCommands,omitempty"`
	Skills               *ContextUsageSkills           `json:"skills,omitempty"`
	AutoCompactThreshold *float64                      `json:"autoCompactThreshold,omitempty"`
	IsAutoCompactEnabled bool                          `json:"isAutoCompactEnabled"`
	MessageBreakdown     *ContextUsageMessageBreakdown `json:"messageBreakdown,omitempty"`
	APIUsage             *ContextUsageAPIUsage         `json:"apiUsage"`
}

// SDKControlGetUsageResponse is the structured data behind the `/usage`
// command: session cost/usage totals plus claude.ai plan rate-limit
// utilization windows.
//
// EXPERIMENTAL: the wire shape is unstable upstream and may change or be
// removed in any release without notice. See Stream.GetUsageExperimental.
type SDKControlGetUsageResponse struct {
	// Session is the cost and usage accumulated by the current session.
	Session UsageSession `json:"session"`

	// SubscriptionType is the claude.ai subscription tier ('pro', 'max',
	// 'team', 'enterprise') or nil for API key / 3P provider sessions.
	SubscriptionType *string `json:"subscription_type"`

	// RateLimitsAvailable is false when plan rate limits do not apply (API
	// key, Bedrock, Vertex, or missing profile scope) — RateLimits is nil.
	RateLimitsAvailable bool `json:"rate_limits_available"`

	// RateLimits holds plan rate-limit utilization windows from the
	// claude.ai usage endpoint, or nil when unavailable.
	RateLimits *UsageRateLimits `json:"rate_limits"`

	// Behaviors describes what's contributing to limits usage, from a scan
	// of local transcripts on this machine. Approximate; nil for
	// non-claude.ai-subscriber sessions or when the scan fails.
	Behaviors *UsageBehaviors `json:"behaviors"`
}

// UsageSession is the per-session cost/usage rollup.
type UsageSession struct {
	TotalCostUSD       float64               `json:"total_cost_usd"`
	TotalAPIDurationMS int                   `json:"total_api_duration_ms"`
	TotalDurationMS    int                   `json:"total_duration_ms"`
	TotalLinesAdded    int                   `json:"total_lines_added"`
	TotalLinesRemoved  int                   `json:"total_lines_removed"`
	ModelUsage         map[string]ModelUsage `json:"model_usage"`
}

// UsageRateLimits holds the plan rate-limit utilization windows. Each window
// is a pointer so absent (nil) and present-but-null are both representable.
type UsageRateLimits struct {
	FiveHour          *UsageRateLimitWindow `json:"five_hour,omitempty"`
	SevenDay          *UsageRateLimitWindow `json:"seven_day,omitempty"`
	SevenDayOAuthApps *UsageRateLimitWindow `json:"seven_day_oauth_apps,omitempty"`
	SevenDayOpus      *UsageRateLimitWindow `json:"seven_day_opus,omitempty"`
	SevenDaySonnet    *UsageRateLimitWindow `json:"seven_day_sonnet,omitempty"`
	// ModelScoped holds per-model weekly windows from the server limits[]
	// array, filtered by the overage-included-models allowlist. Additive —
	// present only when the server emits them. Mirrors sdk.d.ts v0.3.195 L3088.
	ModelScoped []UsageModelScopedWindow `json:"model_scoped,omitempty"`
	ExtraUsage  *UsageExtraUsage         `json:"extra_usage,omitempty"`
}

// UsageModelScopedWindow is a per-model weekly rate-limit window.
type UsageModelScopedWindow struct {
	// DisplayName is the server-supplied label for the model bucket (e.g.
	// "Fable").
	DisplayName string `json:"display_name"`
	// Utilization is the percentage of the window used, or nil.
	Utilization *float64 `json:"utilization"`
	// ResetsAt is the ISO 8601 timestamp when the window resets, or nil.
	ResetsAt *string `json:"resets_at"`
}

// UsageRateLimitWindow is a single rate-limit window's utilization.
type UsageRateLimitWindow struct {
	// Utilization is the percentage of the window used, 0-100, or nil.
	Utilization *float64 `json:"utilization"`
	// ResetsAt is the ISO 8601 timestamp when the window resets, or nil.
	ResetsAt *string `json:"resets_at"`
}

// UsageExtraUsage describes overage/extra-usage credit state.
type UsageExtraUsage struct {
	IsEnabled    bool     `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"`
	UsedCredits  *float64 `json:"used_credits"`
	Utilization  *float64 `json:"utilization"`
	Currency     *string  `json:"currency,omitempty"`
}

// UsageBehaviors holds the day/week behavioral attribution scanned from local
// transcripts.
type UsageBehaviors struct {
	Day  UsageBehaviorsWindow `json:"day"`  // Last 24 hours.
	Week UsageBehaviorsWindow `json:"week"` // Last 7 days.
}

// UsageBehaviorsWindow is the attribution for a single time window.
type UsageBehaviorsWindow struct {
	RequestCount int                     `json:"request_count"`
	SessionCount int                     `json:"session_count"`
	Behaviors    []UsageBehaviorEntry    `json:"behaviors"`
	Agents       []UsageAttributionEntry `json:"agents"`
	Skills       []UsageAttributionEntry `json:"skills"`
	Plugins      []UsageAttributionEntry `json:"plugins"`
	McpServers   []UsageAttributionEntry `json:"mcp_servers"`
}

// UsageBehaviorEntry is one behavioral characteristic. Categories overlap —
// percentages do not sum to 100.
type UsageBehaviorEntry struct {
	// Key is one of cache_miss, long_context, subagent_heavy, high_parallel,
	// cron. Open string for forward-compat with new categories on the wire.
	Key string `json:"key"`
	// Pct is the share of weighted local usage for this behavior, 0-100.
	Pct float64 `json:"pct"`
	// Count is requests in the window exhibiting the behavior.
	Count int `json:"count"`
}

// UsageAttributionEntry attributes a share of local usage to a named
// agent/skill/plugin/MCP server.
type UsageAttributionEntry struct {
	Name string  `json:"name"`
	Pct  float64 `json:"pct"` // Share of weighted local usage, 0-100.
}

type ContextUsageCategory struct {
	Name       string `json:"name"`
	Tokens     int    `json:"tokens"`
	Color      string `json:"color"`
	IsDeferred bool   `json:"isDeferred,omitempty"`
}

type ContextUsageGridCell struct {
	Color          string  `json:"color"`
	IsFilled       bool    `json:"isFilled"`
	CategoryName   string  `json:"categoryName"`
	Tokens         int     `json:"tokens"`
	Percentage     float64 `json:"percentage"`
	SquareFullness float64 `json:"squareFullness"`
}

type ContextUsageMemoryFile struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Tokens int    `json:"tokens"`
}

type ContextUsageMcpTool struct {
	Name       string `json:"name"`
	ServerName string `json:"serverName"`
	Tokens     int    `json:"tokens"`
	IsLoaded   bool   `json:"isLoaded,omitempty"`
}

type ContextUsageBuiltinTool struct {
	Name     string `json:"name"`
	Tokens   int    `json:"tokens"`
	IsLoaded bool   `json:"isLoaded"`
}

type ContextUsageSystemTool struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

type ContextUsageSection struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

type ContextUsageAgent struct {
	AgentType string `json:"agentType"`
	Source    string `json:"source"`
	Tokens    int    `json:"tokens"`
}

type ContextUsageSlashCommands struct {
	TotalCommands    int `json:"totalCommands"`
	IncludedCommands int `json:"includedCommands"`
	Tokens           int `json:"tokens"`
}

type ContextUsageSkills struct {
	TotalSkills      int                       `json:"totalSkills"`
	IncludedSkills   int                       `json:"includedSkills"`
	Tokens           int                       `json:"tokens"`
	SkillFrontmatter []ContextUsageSkillSource `json:"skillFrontmatter"`
}

type ContextUsageSkillSource struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Tokens int    `json:"tokens"`
}

type ContextUsageMessageBreakdown struct {
	ToolCallTokens          int                        `json:"toolCallTokens"`
	ToolResultTokens        int                        `json:"toolResultTokens"`
	AttachmentTokens        int                        `json:"attachmentTokens"`
	AssistantMessageTokens  int                        `json:"assistantMessageTokens"`
	UserMessageTokens       int                        `json:"userMessageTokens"`
	RedirectedContextTokens int                        `json:"redirectedContextTokens"`
	UnattributedTokens      int                        `json:"unattributedTokens"`
	ToolCallsByType         []ContextUsageToolCallType `json:"toolCallsByType"`
	AttachmentsByType       []ContextUsageAttachment   `json:"attachmentsByType"`
}

type ContextUsageToolCallType struct {
	Name         string `json:"name"`
	CallTokens   int    `json:"callTokens"`
	ResultTokens int    `json:"resultTokens"`
}

type ContextUsageAttachment struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

type ContextUsageAPIUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}
