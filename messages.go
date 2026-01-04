package claudeagent

import (
	"encoding/json"
	"time"
)

// Message is the base interface for all messages exchanged with Claude Code CLI.
//
// Messages can be user prompts, assistant responses, control protocol requests,
// streaming events, or result notifications. The MessageType method returns a
// string identifier used for routing and serialization.
type Message interface {
	MessageType() string
}

// UserMessage represents a user prompt sent to Claude.
//
// This message type initiates or continues a conversation. The ParentToolUseID
// field links this message to a specific tool call when providing tool results.
type UserMessage struct {
	Type            string         `json:"type"`                      // Always "user"
	UUID            string         `json:"uuid,omitempty"`            // Unique message ID
	SessionID       string         `json:"session_id"`                // Session identifier
	Message         APIUserMessage `json:"message"`                   // Message content
	ParentToolUseID *string        `json:"parent_tool_use_id"`        // For tool results (null if not tool result)
	IsSynthetic     bool           `json:"isSynthetic,omitempty"`     // True for system-generated messages
	ToolUseResult   interface{}    `json:"tool_use_result,omitempty"` // Tool result JSON if applicable
}

// APIUserMessage represents the message content in Anthropic API format.
type APIUserMessage struct {
	Role    string             `json:"role"`    // Always "user"
	Content []UserContentBlock `json:"content"` // Array of content blocks
}

// UserContentBlock represents a content block in a user message.
type UserContentBlock struct {
	Type string `json:"type"`           // "text" or other types
	Text string `json:"text,omitempty"` // Text content
}

// UserMessageReplay represents a replayed user message during session resume.
type UserMessageReplay struct {
	Type            string         `json:"type"`       // Always "user"
	UUID            string         `json:"uuid"`       // Unique message ID
	SessionID       string         `json:"session_id"` // Session identifier
	Message         APIUserMessage `json:"message"`    // Message content
	ParentToolUseID *string        `json:"parent_tool_use_id"`
	IsReplay        bool           `json:"isReplay"` // True for replayed messages
}

// MessageType implements Message.
func (m UserMessage) MessageType() string { return "user" }

// MessageType implements Message.
func (m UserMessageReplay) MessageType() string { return "user" }

// AssistantMessage represents a response from Claude.
//
// Assistant messages contain one or more content blocks that can be text,
// tool use requests, or thinking blocks. Each message includes usage
// information for billing and rate limiting.
type AssistantMessage struct {
	Type      string `json:"type"`                 // Always "assistant"
	UUID      string `json:"uuid,omitempty"`       // Unique message ID
	SessionID string `json:"session_id,omitempty"` // Session identifier
	Message   struct {
		Role    string         `json:"role"`    // Always "assistant"
		Content []ContentBlock `json:"content"` // Response content blocks
	} `json:"message"`
	ParentToolUseID *string `json:"parent_tool_use_id,omitempty"` // Parent tool use if in subagent
	Usage           *Usage  `json:"usage,omitempty"`              // Token usage for this message
}

// MessageType implements Message.
func (m AssistantMessage) MessageType() string { return "assistant" }

// ContentText returns the concatenated text from all text content blocks.
//
// This is a convenience method for extracting the main text response,
// ignoring tool use and thinking blocks.
func (m AssistantMessage) ContentText() string {
	var text string
	for _, block := range m.Message.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return text
}

// ContentBlock represents a single content element in an assistant message.
//
// Content blocks can be:
// - text: Plain text response
// - tool_use: Request to execute a tool
// - thinking: Claude's reasoning process (when extended thinking is enabled)
type ContentBlock struct {
	Type  string          `json:"type"`            // "text", "tool_use", or "thinking"
	Text  string          `json:"text,omitempty"`  // For text and thinking blocks
	ID    string          `json:"id,omitempty"`    // For tool_use blocks (unique ID)
	Name  string          `json:"name,omitempty"`  // For tool_use blocks (tool name)
	Input json.RawMessage `json:"input,omitempty"` // For tool_use blocks (arguments)
}

// BlockType returns the type of this content block.
func (c ContentBlock) BlockType() string { return c.Type }

// ResultMessage represents the final outcome of a conversation turn.
//
// This message signals completion (success or error) and includes cumulative
// usage statistics for the entire interaction.
type ResultMessage struct {
	Type   string `json:"type"`   // Always "result"
	Status string `json:"status"` // "success" or "error" (deprecated: use Subtype)

	// Subtype indicates the result type (TypeScript SDK compatible).
	// Values: "success", "error_max_turns", "error_during_execution",
	// "error_max_budget_usd", "error_max_structured_output_retries"
	Subtype string `json:"subtype,omitempty"`

	UUID      string `json:"uuid,omitempty"`       // Unique message ID
	SessionID string `json:"session_id,omitempty"` // Session identifier

	Result string   `json:"result,omitempty"` // Result text (for success)
	Errors []string `json:"errors,omitempty"` // Error messages (for errors)

	DurationMs    int64 `json:"duration_ms,omitempty"`     // Total duration in milliseconds
	DurationAPIMs int64 `json:"duration_api_ms,omitempty"` // API call duration in milliseconds
	IsError       bool  `json:"is_error,omitempty"`        // Whether this is an error result
	NumTurns      int   `json:"num_turns,omitempty"`       // Number of conversation turns

	TotalCostUSD float64 `json:"total_cost_usd,omitempty"` // Total cost in USD

	Usage      *NonNullableUsage     `json:"usage,omitempty"`      // Token usage
	ModelUsage map[string]ModelUsage `json:"modelUsage,omitempty"` // Per-model usage

	PermissionDenials []PermissionDenial `json:"permission_denials,omitempty"` // Denied permissions
	StructuredOutput  interface{}        `json:"structured_output,omitempty"`  // Structured output (if OutputFormat set)
}

// MessageType implements Message.
func (m ResultMessage) MessageType() string { return "result" }

// StreamEvent represents a progressive delta update during streaming.
//
// Stream events allow real-time display of Claude's response as it's generated.
// The Event field indicates whether this is a delta (partial update) or done
// (streaming complete for this message).
type StreamEvent struct {
	Type      string    `json:"type"`  // Always "stream_event"
	Event     string    `json:"event"` // "delta" or "done"
	Delta     string    `json:"delta,omitempty"`
	Timestamp time.Time `json:"timestamp"` // Event timestamp
}

// MessageType implements Message.
func (m StreamEvent) MessageType() string { return "stream_event" }

// TodoUpdateMessage contains task tracking updates from Claude.
//
// Claude automatically creates and updates todos for complex multi-step tasks.
// This message type allows clients to display progress tracking UI.
type TodoUpdateMessage struct {
	Type  string     `json:"type"` // Always "todo_update"
	Items []TodoItem `json:"items"`
}

// MessageType implements Message.
func (m TodoUpdateMessage) MessageType() string { return "todo_update" }

// TodoItem represents a single task in Claude's task list.
//
// Each item has two forms: Content (imperative: "Run tests") and ActiveForm
// (continuous: "Running tests"). The Status field tracks lifecycle state.
type TodoItem struct {
	Content    string     `json:"content"`    // Task description (imperative form)
	ActiveForm string     `json:"activeForm"` // In-progress form (continuous)
	Status     TodoStatus `json:"status"`     // Lifecycle state
}

// TodoStatus represents the lifecycle state of a todo item.
type TodoStatus string

const (
	// TodoStatusPending indicates the task has not started.
	TodoStatusPending TodoStatus = "pending"

	// TodoStatusInProgress indicates the task is currently being worked on.
	TodoStatusInProgress TodoStatus = "in_progress"

	// TodoStatusCompleted indicates the task has finished.
	TodoStatusCompleted TodoStatus = "completed"
)

// SubagentResultMessage contains the result of a subagent invocation.
//
// When Claude delegates work to a specialized subagent, this message
// communicates the outcome back to the main agent.
type SubagentResultMessage struct {
	Type      string `json:"type"`       // Always "subagent_result"
	AgentName string `json:"agent_name"` // Subagent identifier
	Status    string `json:"status"`     // "success" or "error"
	Result    string `json:"result"`     // Subagent output
}

// MessageType implements Message.
func (m SubagentResultMessage) MessageType() string { return "subagent_result" }

// SDKControlRequest represents a control protocol request sent from SDK to CLI.
//
// Control requests are used for initialization, permission checks, hook
// invocations, and other SDK-level coordination. Each request has a unique
// ID for correlation with responses.
type SDKControlRequest struct {
	Type      string                `json:"type"`       // Always "control_request"
	RequestID string                `json:"request_id"` // Unique request ID (snake_case)
	Request   SDKControlRequestBody `json:"request"`    // Nested request payload
}

// SDKControlRequestBody contains the actual request data.
// Note: This is a union type - different fields are used for different subtypes.
type SDKControlRequestBody struct {
	Subtype            string                              `json:"subtype"`                       // Request subtype
	Hooks              map[string][]SDKHookCallbackMatcher `json:"hooks,omitempty"`               // For initialize
	SDKMCPServers      []string                            `json:"sdkMcpServers,omitempty"`       // For initialize
	JSONSchema         map[string]interface{}              `json:"jsonSchema,omitempty"`          // For initialize
	SystemPrompt       string                              `json:"systemPrompt,omitempty"`        // For initialize
	AppendSystemPrompt string                              `json:"appendSystemPrompt,omitempty"`  // For initialize
	Agents             map[string]interface{}              `json:"agents,omitempty"`              // For initialize
	ToolName           string                              `json:"tool_name,omitempty"`           // For can_use_tool/hook_callback
	Input              map[string]interface{}              `json:"input,omitempty"`               // For can_use_tool/hook_callback
	ToolUseID          string                              `json:"tool_use_id,omitempty"`         // For can_use_tool/hooks
	AgentID            string                              `json:"agent_id,omitempty"`            // For can_use_tool
	CallbackID         string                              `json:"callback_id,omitempty"`         // For hook_callback
	Mode               string                              `json:"mode,omitempty"`                // For set_permission_mode
	Model              string                              `json:"model,omitempty"`               // For set_model
	MaxThinkingTokens  *int                                `json:"max_thinking_tokens,omitempty"` // For set_max_thinking_tokens
	UserMessageID      string                              `json:"user_message_id,omitempty"`     // For rewind_files
	ServerName         string                              `json:"server_name,omitempty"`         // For mcp_message
	Message            map[string]interface{}              `json:"message,omitempty"`             // For mcp_message (JSONRPC)
}

// SDKHookCallbackMatcher defines hook callback matching configuration.
type SDKHookCallbackMatcher struct {
	Matcher         string   `json:"matcher,omitempty"`
	HookCallbackIDs []string `json:"hookCallbackIds"`
	Timeout         int      `json:"timeout,omitempty"` // Timeout in seconds
}

// MessageType implements Message.
func (m SDKControlRequest) MessageType() string { return "control_request" }

// SDKControlResponse represents a control protocol response from CLI to SDK.
//
// Control responses correlate to requests via RequestID and contain either
// a result payload or an error.
type SDKControlResponse struct {
	Type     string                 `json:"type"`     // Always "control_response"
	Response SDKControlResponseBody `json:"response"` // Nested response payload
}

// SDKControlResponseBody contains the actual response data.
type SDKControlResponseBody struct {
	Subtype                   string                 `json:"subtype"`                               // "success" or "error"
	RequestID                 string                 `json:"request_id"`                            // Correlates to request
	Response                  map[string]interface{} `json:"response,omitempty"`                    // Success response data
	Error                     string                 `json:"error,omitempty"`                       // Error message
	PendingPermissionRequests []SDKControlRequest    `json:"pending_permission_requests,omitempty"` // Pending requests
}

// MessageType implements Message.
func (m SDKControlResponse) MessageType() string { return "control_response" }

// SDKControlCancelRequest cancels a pending control request.
type SDKControlCancelRequest struct {
	Type      string `json:"type"`       // Always "control_cancel_request"
	RequestID string `json:"request_id"` // Request to cancel
}

// MessageType implements Message.
func (m SDKControlCancelRequest) MessageType() string { return "control_cancel_request" }

// KeepAliveMessage is a heartbeat message.
type KeepAliveMessage struct {
	Type string `json:"type"` // Always "keep_alive"
}

// MessageType implements Message.
func (m KeepAliveMessage) MessageType() string { return "keep_alive" }

// ToolProgressMessage reports tool execution progress.
type ToolProgressMessage struct {
	Type               string  `json:"type"`                 // Always "tool_progress"
	ToolUseID          string  `json:"tool_use_id"`          // Tool invocation ID
	ToolName           string  `json:"tool_name"`            // Tool name
	ParentToolUseID    *string `json:"parent_tool_use_id"`   // Parent tool if nested
	ElapsedTimeSeconds float64 `json:"elapsed_time_seconds"` // Time elapsed
	UUID               string  `json:"uuid"`                 // Message UUID
	SessionID          string  `json:"session_id"`           // Session ID
}

// MessageType implements Message.
func (m ToolProgressMessage) MessageType() string { return "tool_progress" }

// AuthStatusMessage reports authentication status.
type AuthStatusMessage struct {
	Type             string   `json:"type"`             // Always "auth_status"
	IsAuthenticating bool     `json:"isAuthenticating"` // Whether auth is in progress
	Output           []string `json:"output"`           // Auth output messages
	Error            string   `json:"error,omitempty"`  // Error if any
	UUID             string   `json:"uuid"`             // Message UUID
	SessionID        string   `json:"session_id"`       // Session ID
}

// MessageType implements Message.
func (m AuthStatusMessage) MessageType() string { return "auth_status" }

// Legacy ControlRequest/ControlResponse kept for backward compatibility.
// These may be used internally but SDKControlRequest/SDKControlResponse
// should be used for CLI communication.

// ControlRequest represents a legacy control protocol request.
type ControlRequest struct {
	Type      string                 `json:"type"`              // "control"
	Subtype   string                 `json:"subtype"`           // Request subtype
	RequestID string                 `json:"requestId"`         // Unique request ID
	Payload   map[string]interface{} `json:"payload,omitempty"` // Request data
}

// MessageType implements Message.
func (m ControlRequest) MessageType() string { return "control" }

// ControlResponse represents a legacy control protocol response.
type ControlResponse struct {
	Type      string                 `json:"type"`             // "control"
	RequestID string                 `json:"requestId"`        // Correlates to request
	Result    map[string]interface{} `json:"result,omitempty"` // Response data
	Error     *ProtocolError         `json:"error,omitempty"`  // Error details
}

// MessageType implements Message.
func (m ControlResponse) MessageType() string { return "control" }

// ProtocolError represents an error in the control protocol.
type ProtocolError struct {
	Code    string `json:"code"`    // Error code
	Message string `json:"message"` // Human-readable message
}

// Usage tracks token consumption and cost for billing.
//
// Usage data appears in assistant messages (per-message) and result messages
// (cumulative). Token counts distinguish between input (prompt) and output
// (completion) tokens.
type Usage struct {
	InputTokens  int     `json:"input_tokens"`  // Prompt tokens
	OutputTokens int     `json:"output_tokens"` // Completion tokens
	TotalTokens  int     `json:"total_tokens"`  // Sum of input + output
	Cost         float64 `json:"cost"`          // Estimated cost in USD
}

// SystemMessage represents the initialization message from Claude Code.
//
// This message is sent at the start of a session and contains information
// about available tools, MCP servers, models, and permissions.
type SystemMessage struct {
	Type           string          `json:"type"`           // Always "system"
	Subtype        string          `json:"subtype"`        // "init" or "compact_boundary"
	UUID           string          `json:"uuid"`           // Unique message ID
	SessionID      string          `json:"session_id"`     // Session identifier
	APIKeySource   string          `json:"apiKeySource"`   // Where the API key comes from
	Cwd            string          `json:"cwd"`            // Current working directory
	Tools          []string        `json:"tools"`          // Available tools
	MCPServers     []MCPServerInfo `json:"mcp_servers"`    // MCP server status
	Model          string          `json:"model"`          // Active model
	PermissionMode PermissionMode  `json:"permissionMode"` // Current permission mode
	SlashCommands  []string        `json:"slash_commands"` // Available slash commands
	OutputStyle    string          `json:"output_style"`   // Output formatting style
}

// MessageType implements Message.
func (m SystemMessage) MessageType() string { return "system" }

// MCPServerInfo contains status information about an MCP server.
type MCPServerInfo struct {
	Name   string `json:"name"`   // Server name
	Status string `json:"status"` // Connection status
}

// PartialAssistantMessage represents a streaming partial message.
//
// These messages are only emitted when IncludePartialMessages is true in Options.
// They contain raw streaming events for real-time display.
type PartialAssistantMessage struct {
	Type            string          `json:"type"`  // Always "stream_event"
	Event           json.RawMessage `json:"event"` // Raw streaming event
	ParentToolUseID *string         `json:"parent_tool_use_id,omitempty"`
	UUID            string          `json:"uuid"`       // Unique message ID
	SessionID       string          `json:"session_id"` // Session identifier
}

// MessageType implements Message.
func (m PartialAssistantMessage) MessageType() string { return "stream_event" }

// CompactBoundaryMessage marks a context compaction boundary.
//
// This message is emitted when context compaction occurs, either manually
// or automatically when approaching context limits.
type CompactBoundaryMessage struct {
	Type            string          `json:"type"`             // Always "system"
	Subtype         string          `json:"subtype"`          // "compact_boundary"
	UUID            string          `json:"uuid"`             // Unique message ID
	SessionID       string          `json:"session_id"`       // Session identifier
	CompactMetadata CompactMetadata `json:"compact_metadata"` // Compaction details
}

// MessageType implements Message.
func (m CompactBoundaryMessage) MessageType() string { return "system" }

// CompactMetadata contains details about a compaction event.
type CompactMetadata struct {
	Trigger   string `json:"trigger"`    // "manual" or "auto"
	PreTokens int    `json:"pre_tokens"` // Token count before compaction
}

// PermissionDenial tracks a denied permission request.
type PermissionDenial struct {
	ToolName  string          `json:"tool_name"`  // Tool that was denied
	ToolInput json.RawMessage `json:"tool_input"` // Input that triggered denial
	Reason    string          `json:"reason"`     // Why permission was denied
}

// ModelUsage tracks usage statistics per model.
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens"`              // Prompt tokens
	OutputTokens             int     `json:"outputTokens"`             // Completion tokens
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`     // Cache read tokens
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"` // Cache creation tokens
	WebSearchRequests        int     `json:"webSearchRequests"`        // Web search count
	CostUSD                  float64 `json:"costUSD"`                  // Cost in USD
	ContextWindow            int     `json:"contextWindow"`            // Context window size
}

// NonNullableUsage is like Usage but all fields are guaranteed non-zero.
type NonNullableUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

// ParseMessage parses a JSON message into the appropriate Message type.
//
// This function inspects the "type" field to determine the concrete type
// and unmarshals accordingly. Unknown types return an error.
func ParseMessage(data []byte) (Message, error) {
	// First, peek at the type field
	var typeOnly struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typeOnly); err != nil {
		return nil, err
	}

	// Unmarshal into the appropriate concrete type
	switch typeOnly.Type {
	case "user":
		var msg UserMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "assistant":
		var msg AssistantMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "result":
		var msg ResultMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "stream_event":
		// Could be StreamEvent or PartialAssistantMessage
		var partial PartialAssistantMessage
		if err := json.Unmarshal(data, &partial); err == nil && partial.UUID != "" {
			return partial, nil
		}
		var msg StreamEvent
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "system":
		// System messages have subtypes: "init" or "compact_boundary"
		var base struct {
			Subtype string `json:"subtype"`
		}
		if err := json.Unmarshal(data, &base); err != nil {
			return nil, err
		}

		if base.Subtype == "compact_boundary" {
			var msg CompactBoundaryMessage
			err := json.Unmarshal(data, &msg)
			return msg, err
		}

		// Default: init message
		var msg SystemMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "todo_update":
		var msg TodoUpdateMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "subagent_result":
		var msg SubagentResultMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "control_request":
		var msg SDKControlRequest
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "control_response":
		var msg SDKControlResponse
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "control_cancel_request":
		var msg SDKControlCancelRequest
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "keep_alive":
		return KeepAliveMessage{Type: "keep_alive"}, nil

	case "tool_progress":
		var msg ToolProgressMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "auth_status":
		var msg AuthStatusMessage
		err := json.Unmarshal(data, &msg)
		return msg, err

	case "control":
		// Legacy control messages - determine which by checking for requestId or result
		var base struct {
			Subtype   string          `json:"subtype,omitempty"`
			RequestID string          `json:"requestId"`
			Result    json.RawMessage `json:"result,omitempty"`
		}
		if err := json.Unmarshal(data, &base); err != nil {
			return nil, err
		}

		// If it has a subtype, it's a request
		if base.Subtype != "" {
			var msg ControlRequest
			err := json.Unmarshal(data, &msg)
			return msg, err
		}

		// Otherwise it's a response
		var msg ControlResponse
		err := json.Unmarshal(data, &msg)
		return msg, err

	default:
		return nil, &ErrUnknownMessageType{Type: typeOnly.Type}
	}
}
