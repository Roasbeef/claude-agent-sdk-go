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

// AccountInfo contains user account information.
type AccountInfo struct {
	Email            string `json:"email,omitempty"`            // User email
	Organization     string `json:"organization,omitempty"`     // Organization name
	SubscriptionType string `json:"subscriptionType,omitempty"` // Subscription tier
	TokenSource      string `json:"tokenSource,omitempty"`      // How token was obtained
	APIKeySource     string `json:"apiKeySource,omitempty"`     // API key source
}
