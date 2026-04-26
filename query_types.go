package claudeagent

// SlashCommand represents an available slash command.
type SlashCommand struct {
	Name         string `json:"name"`         // Command name (without slash)
	Description  string `json:"description"`  // Command description
	ArgumentHint string `json:"argumentHint"` // Hint for command arguments
}

// ModelInfo contains information about an available model.
type ModelInfo struct {
	Value       string `json:"value"`       // Model ID to use in API calls
	DisplayName string `json:"displayName"` // Human-readable model name
	Description string `json:"description"` // Model capabilities description
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

// AccountInfo contains user account information.
type AccountInfo struct {
	Email            string `json:"email,omitempty"`            // User email
	Organization     string `json:"organization,omitempty"`     // Organization name
	SubscriptionType string `json:"subscriptionType,omitempty"` // Subscription tier
	TokenSource      string `json:"tokenSource,omitempty"`      // How token was obtained
	APIKeySource     string `json:"apiKeySource,omitempty"`     // API key source
	APIProvider      string `json:"apiProvider,omitempty"`      // Active API backend
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
