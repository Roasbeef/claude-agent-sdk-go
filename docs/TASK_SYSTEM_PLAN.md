# Task System Implementation Plan

This document outlines the plan for exposing the Task system in the Go SDK, based on analysis of the TypeScript SDK's implementation (version 0.2.30, CLI v2.1.30).

> **Note:** Originally based on v0.2.17, updated for v0.2.30 which adds `name`, `team_name`, and `mode` fields to `AgentInput`, the full `PermissionMode` enum (`default`, `acceptEdits`, `bypassPermissions`, `plan`, `delegate`, `dontAsk`), new hook events (`PermissionRequest`, `Setup`), and new SDK options (`enableFileCheckpointing`, `forkSession`, `persistSession`, `maxBudgetUsd`, `fallbackModel`, `betas`).

## Executive Summary

The Task system enables Claude to spawn specialized subagents for delegating work. Key capabilities include:

1. **Programmatic Agent Definitions** - Define custom subagents with specific tools, prompts, and models
2. **Background Task Execution** - Run tasks asynchronously with status notifications
3. **Task Output Retrieval** - Poll or block for background task results
4. **Agent Lifecycle Hooks** - Intercept subagent start/stop events
5. **Named Agent Spawning** - Spawn agents with explicit names and team context (v0.2.30)
6. **Permission Mode Delegation** - Control subagent permission behavior via `mode` field (v0.2.30)

The Go SDK already has partial support (agent definitions, subagent hooks), but lacks:
- `TaskNotificationMessage` for background task completion
- `TaskOutput` tool input types for retrieving results
- High-level APIs for task management

---

## TypeScript SDK Reference

### Core Types

```typescript
// Agent definition (already partially in Go SDK)
type AgentDefinition = {
  description: string;              // When to use this agent
  tools?: string[];                 // Allowed tools (nil = inherit all)
  disallowedTools?: string[];       // Explicitly blocked tools
  prompt: string;                   // System prompt
  model?: 'sonnet' | 'opus' | 'haiku' | 'inherit';
  mcpServers?: AgentMcpServerSpec[];
  skills?: string[];                // Skills to preload
  maxTurns?: number;                // API round-trip limit
  criticalSystemReminder_EXPERIMENTAL?: string;
};

// Task tool input (Agent/Task tool) - v0.2.30
interface AgentInput {
  description: string;              // Short task description (3-5 words)
  prompt: string;                   // Task for agent to perform
  subagent_type: string;            // Agent type to use
  model?: 'sonnet' | 'opus' | 'haiku';
  resume?: string;                  // Agent ID to resume
  run_in_background?: boolean;      // Async execution
  max_turns?: number;               // Turn limit
  name?: string;                    // Agent name (v0.2.30)
  team_name?: string;               // Team context (v0.2.30)
  mode?: PermissionMode;            // Permission mode (v0.2.30)
}

// Permission modes for subagent spawning - v0.2.30
type PermissionMode =
  | 'default'            // Standard behavior, prompts for dangerous operations
  | 'acceptEdits'        // Auto-accept file edit operations
  | 'bypassPermissions'  // Bypass all permission checks
  | 'plan'               // Planning mode, requires plan approval
  | 'delegate'           // Delegate permission decisions to parent
  | 'dontAsk';           // Don't prompt, deny if not pre-approved

// Task output retrieval
interface TaskOutputInput {
  task_id: string;                  // Task to get output from
  block: boolean;                   // Wait for completion
  timeout: number;                  // Max wait time (ms)
}

// Background task notification
type SDKTaskNotificationMessage = {
  type: 'system';
  subtype: 'task_notification';
  task_id: string;
  status: 'completed' | 'failed' | 'stopped';
  output_file: string;
  summary: string;
  uuid: UUID;
  session_id: string;
};
```

### Subagent Hooks

```typescript
// Already in Go SDK
type SubagentStartHookInput = BaseHookInput & {
  hook_event_name: 'SubagentStart';
  agent_id: string;
  agent_type: string;
};

type SubagentStopHookInput = BaseHookInput & {
  hook_event_name: 'SubagentStop';
  stop_hook_active: boolean;
  agent_id: string;
  agent_transcript_path: string;
};

// New hook events in v0.2.30
type PermissionRequestHookInput = BaseHookInput & {
  hook_event_name: 'PermissionRequest';
  tool_name: string;
  tool_input: unknown;
  permission_suggestions?: PermissionUpdate[];
};

type SetupHookInput = BaseHookInput & {
  hook_event_name: 'Setup';
};
```

### Additional v0.2.30 SDK Options

```typescript
// New ClaudeCodeOptions fields
interface ClaudeCodeOptions {
  // ... existing fields ...
  agent?: string;                    // Run as a specific agent type
  enableFileCheckpointing?: boolean; // Track file changes for rewinding
  forkSession?: boolean;             // Fork resumed sessions to new ID
  betas?: SdkBeta[];                 // Enable beta features (e.g., 'context-1m-2025-08-07')
  persistSession?: boolean;          // Disable session persistence (default: true)
  maxBudgetUsd?: number;             // Maximum budget in USD
  fallbackModel?: string;            // Fallback model if primary fails
  tools?: string[] | { type: 'preset'; preset: 'claude_code' }; // Restrict available tools
}
```

---

## Implementation Plan

### Phase 1: Message Types & Parsing

**File: `messages.go`**

Add `TaskNotificationMessage` type:

```go
// TaskNotificationMessage signals completion of a background task.
//
// When Claude spawns a task with run_in_background=true, this message
// is emitted when the task completes, fails, or is stopped.
type TaskNotificationMessage struct {
    Type       string     `json:"type"`       // Always "system"
    Subtype    string     `json:"subtype"`    // Always "task_notification"
    TaskID     string     `json:"task_id"`    // Unique task identifier
    Status     TaskStatus `json:"status"`     // completed, failed, stopped
    OutputFile string     `json:"output_file"`// Path to task output
    Summary    string     `json:"summary"`    // Task result summary
    UUID       string     `json:"uuid"`       // Message UUID
    SessionID  string     `json:"session_id"` // Session identifier
}

// TaskStatus represents the completion state of a background task.
type TaskStatus string

const (
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusStopped   TaskStatus = "stopped"
)

// MessageType implements Message.
func (m TaskNotificationMessage) MessageType() string { return "system" }

// IsSuccess returns true if the task completed successfully.
func (m TaskNotificationMessage) IsSuccess() bool {
    return m.Status == TaskStatusCompleted
}
```

Update `ParseMessage()` to handle task notifications:

```go
case "system":
    var base struct {
        Subtype string `json:"subtype"`
    }
    if err := json.Unmarshal(data, &base); err != nil {
        return nil, err
    }

    switch base.Subtype {
    case "task_notification":
        var msg TaskNotificationMessage
        err := json.Unmarshal(data, &msg)
        return msg, err
    case "compact_boundary":
        // existing code
    default:
        // existing init handling
    }
```

### Phase 2: Tool Input Types

**File: `tool_inputs.go`**

Add Task-related tool input types:

```go
// TaskInput represents the input for the Task tool (v0.2.30).
//
// Claude uses this to spawn subagents for delegated work. The subagent_type
// determines which agent definition to use.
type TaskInput struct {
    // Description is a short (3-5 word) summary of the task.
    Description string `json:"description"`

    // Prompt is the detailed task for the agent to perform.
    Prompt string `json:"prompt"`

    // SubagentType specifies which agent definition to use.
    SubagentType string `json:"subagent_type"`

    // Model overrides the default model for this task.
    // Options: "sonnet", "opus", "haiku", or empty for inherit.
    Model string `json:"model,omitempty"`

    // Resume specifies an agent ID to continue from.
    Resume string `json:"resume,omitempty"`

    // RunInBackground enables async execution.
    // When true, the tool returns immediately with task_id and output_file.
    RunInBackground bool `json:"run_in_background,omitempty"`

    // MaxTurns limits API round-trips for the subagent.
    MaxTurns *int `json:"max_turns,omitempty"`

    // Name labels the spawned agent for tracking and identification (v0.2.30).
    Name string `json:"name,omitempty"`

    // TeamName specifies team context for spawning. Uses current team
    // context if omitted (v0.2.30).
    TeamName string `json:"team_name,omitempty"`

    // Mode sets the permission mode for the subagent (v0.2.30).
    // Options: "default", "acceptEdits", "bypassPermissions", "plan",
    // "delegate", "dontAsk".
    Mode PermissionMode `json:"mode,omitempty"`
}

// TaskOutputInput retrieves results from a background task.
type TaskOutputInput struct {
    // TaskID is the task to retrieve output from.
    TaskID string `json:"task_id"`

    // Block waits for task completion if true.
    Block bool `json:"block"`

    // Timeout is the maximum wait time in milliseconds.
    Timeout int `json:"timeout"`
}
```

### Phase 3: Enhanced Agent Definition

**File: `options.go`**

Expand `AgentDefinition` to match TypeScript SDK:

```go
// AgentDefinition defines a specialized subagent for task delegation.
//
// Agents are invoked via the Task tool. Claude selects the appropriate
// agent based on task context and agent descriptions.
type AgentDefinition struct {
    // Name is the agent identifier (used as map key).
    Name string `json:"-"`

    // Description explains when to invoke this agent.
    // Claude uses this to determine task routing.
    Description string `json:"description"`

    // Prompt is the system prompt for the subagent.
    Prompt string `json:"prompt"`

    // Tools restricts available tools. Nil inherits all tools.
    Tools []string `json:"tools,omitempty"`

    // DisallowedTools explicitly blocks specific tools.
    DisallowedTools []string `json:"disallowedTools,omitempty"`

    // Model overrides the default model.
    // Options: "sonnet", "opus", "haiku", "inherit", or empty.
    Model string `json:"model,omitempty"`

    // MCPServers configures MCP servers for this agent.
    MCPServers []AgentMCPServerSpec `json:"mcpServers,omitempty"`

    // Skills preloads specific skills into agent context.
    Skills []string `json:"skills,omitempty"`

    // MaxTurns limits API round-trips.
    MaxTurns *int `json:"maxTurns,omitempty"`

    // CriticalSystemReminder is an experimental critical reminder.
    CriticalSystemReminder string `json:"criticalSystemReminder_EXPERIMENTAL,omitempty"`
}

// AgentMCPServerSpec can be a server name or full config.
type AgentMCPServerSpec struct {
    // Name references a pre-configured MCP server.
    Name string `json:"name,omitempty"`

    // Config provides inline server configuration.
    Config *MCPServerConfig `json:"config,omitempty"`
}
```

### Phase 4: High-Level Task API

**File: `tasks.go` (new file)**

Provide ergonomic APIs for task management:

```go
package claudeagent

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
)

// Task represents a spawned background task.
//
// Tasks are created when Claude invokes the Task tool with run_in_background=true.
// Use Wait() to block until completion or Poll() to check status.
type Task struct {
    ID         string
    OutputFile string
    client     *Client
    done       chan TaskNotificationMessage
    result     *TaskNotificationMessage
}

// TaskManager tracks background tasks for a client session.
type TaskManager struct {
    client *Client
    tasks  map[string]*Task
}

// NewTaskManager creates a task manager for tracking background tasks.
func NewTaskManager(client *Client) *TaskManager {
    return &TaskManager{
        client: client,
        tasks:  make(map[string]*Task),
    }
}

// Track registers a task from a TaskNotificationMessage.
//
// Call this when receiving task_notification messages in your message loop.
func (tm *TaskManager) Track(notification TaskNotificationMessage) *Task {
    task, exists := tm.tasks[notification.TaskID]
    if !exists {
        task = &Task{
            ID:         notification.TaskID,
            OutputFile: notification.OutputFile,
            client:     tm.client,
            done:       make(chan TaskNotificationMessage, 1),
        }
        tm.tasks[notification.TaskID] = task
    }

    // Signal completion
    task.result = &notification
    select {
    case task.done <- notification:
    default:
    }

    return task
}

// Get retrieves a tracked task by ID.
func (tm *TaskManager) Get(taskID string) (*Task, bool) {
    task, ok := tm.tasks[taskID]
    return task, ok
}

// Wait blocks until the task completes or context is canceled.
func (t *Task) Wait(ctx context.Context) (TaskNotificationMessage, error) {
    if t.result != nil {
        return *t.result, nil
    }

    select {
    case <-ctx.Done():
        return TaskNotificationMessage{}, ctx.Err()
    case result := <-t.done:
        return result, nil
    }
}

// Status returns the task status, or empty string if still running.
func (t *Task) Status() TaskStatus {
    if t.result != nil {
        return t.result.Status
    }
    return ""
}

// Summary returns the task result summary.
func (t *Task) Summary() string {
    if t.result != nil {
        return t.result.Summary
    }
    return ""
}

// ReadOutput reads the task output file contents.
func (t *Task) ReadOutput(ctx context.Context) (string, error) {
    // Implementation would read from t.OutputFile
    // This requires filesystem access to the output file
    return "", fmt.Errorf("not implemented: read output file %s", t.OutputFile)
}
```

### Phase 5: Client Integration

**File: `client.go`**

Add task notification handling to message pump:

```go
// Query performs a one-shot query and returns an iterator over response messages.
func (c *Client) Query(ctx context.Context, prompt string) iter.Seq[Message] {
    return func(yield func(Message) bool) {
        // ... existing code ...

        for {
            select {
            case <-ctx.Done():
                return
            case msg, ok := <-c.msgCh:
                if !ok {
                    return
                }

                // Handle task notifications
                if taskNotif, ok := msg.(TaskNotificationMessage); ok {
                    if c.options.TaskNotificationHandler != nil {
                        c.options.TaskNotificationHandler(ctx, taskNotif)
                    }
                }

                // ... rest of existing handling ...
            }
        }
    }
}
```

Add option for task notification handler:

```go
// options.go

// TaskNotificationHandler is called when background tasks complete.
type TaskNotificationHandler func(ctx context.Context, notification TaskNotificationMessage)

// WithTaskNotificationHandler sets a callback for background task completion.
func WithTaskNotificationHandler(handler TaskNotificationHandler) Option {
    return func(o *Options) {
        o.TaskNotificationHandler = handler
    }
}
```

### Phase 6: Subagent Hook Enhancement

**File: `options.go`**

Update `SubagentStopInput` to include additional fields:

```go
// SubagentStopInput contains data for SubagentStop hooks.
type SubagentStopInput struct {
    BaseHookInput

    // StopHookActive indicates if the Stop hook is active.
    StopHookActive bool `json:"stop_hook_active,omitempty"`

    // AgentID is the unique identifier of the stopped agent.
    AgentID string `json:"agent_id"`

    // AgentTranscriptPath is the path to the agent's conversation transcript.
    AgentTranscriptPath string `json:"agent_transcript_path"`
}
```

---

## API Design Principles

### 1. Consistent with TypeScript SDK

The Go SDK should mirror TypeScript SDK capabilities while using Go idioms:

| TypeScript | Go |
|------------|-----|
| `agents: Record<string, AgentDefinition>` | `WithAgents(map[string]AgentDefinition)` |
| `for await (const msg of query(...))` | `for msg := range client.Query(...)` |
| `SDKTaskNotificationMessage` | `TaskNotificationMessage` |

### 2. Progressive Disclosure

Simple cases should be simple:

```go
// Basic usage - just define agents
client, _ := claudeagent.NewClient(
    claudeagent.WithAgents(map[string]claudeagent.AgentDefinition{
        "reviewer": {
            Description: "Code review specialist",
            Prompt:      "You review code for quality...",
        },
    }),
)

// Claude automatically routes tasks to appropriate agents
for msg := range client.Query(ctx, "Review the authentication module") {
    // Handle messages
}
```

Advanced usage adds task tracking:

```go
// Advanced usage - track background tasks
taskManager := claudeagent.NewTaskManager(client)

client, _ := claudeagent.NewClient(
    claudeagent.WithTaskNotificationHandler(func(ctx context.Context, notif TaskNotificationMessage) {
        taskManager.Track(notif)
    }),
)

for msg := range client.Query(ctx, "Run these three analyses in background") {
    switch m := msg.(type) {
    case claudeagent.AssistantMessage:
        // Check if this contains task spawning
    case claudeagent.TaskNotificationMessage:
        task := taskManager.Track(m)
        fmt.Printf("Task %s: %s\n", task.ID, task.Status())
    }
}
```

### 3. Type Safety

Use Go's type system to prevent errors:

```go
// Good: Enum for task status
type TaskStatus string
const (
    TaskStatusCompleted TaskStatus = "completed"
    TaskStatusFailed    TaskStatus = "failed"
    TaskStatusStopped   TaskStatus = "stopped"
)

// Good: Type-safe model options
type AgentModel string
const (
    AgentModelSonnet  AgentModel = "sonnet"
    AgentModelOpus    AgentModel = "opus"
    AgentModelHaiku   AgentModel = "haiku"
    AgentModelInherit AgentModel = "inherit"
)
```

### 4. Error Handling

Surface errors appropriately:

```go
// Task-specific errors
type ErrTaskNotFound struct {
    TaskID string
}

func (e *ErrTaskNotFound) Error() string {
    return fmt.Sprintf("task not found: %s", e.TaskID)
}

type ErrTaskTimeout struct {
    TaskID  string
    Timeout time.Duration
}

func (e *ErrTaskTimeout) Error() string {
    return fmt.Sprintf("task %s timed out after %v", e.TaskID, e.Timeout)
}
```

---

## Testing Strategy

### Unit Tests

```go
// messages_test.go
func TestParseTaskNotificationMessage(t *testing.T) {
    data := []byte(`{
        "type": "system",
        "subtype": "task_notification",
        "task_id": "task_123",
        "status": "completed",
        "output_file": "/tmp/output.txt",
        "summary": "Analysis complete",
        "uuid": "uuid-123",
        "session_id": "sess-456"
    }`)

    msg, err := ParseMessage(data)
    require.NoError(t, err)

    taskNotif, ok := msg.(TaskNotificationMessage)
    require.True(t, ok)
    assert.Equal(t, "task_123", taskNotif.TaskID)
    assert.Equal(t, TaskStatusCompleted, taskNotif.Status)
    assert.True(t, taskNotif.IsSuccess())
}
```

### Integration Tests

```go
// integration_test.go
func TestBackgroundTask(t *testing.T) {
    skipIfNoToken(t)

    taskCh := make(chan TaskNotificationMessage, 1)

    client, err := claudeagent.NewClient(
        claudeagent.WithSystemPrompt("Run tasks in background when asked"),
        claudeagent.WithPermissionMode(claudeagent.PermissionModeBypassAll),
        claudeagent.WithTaskNotificationHandler(func(ctx context.Context, notif TaskNotificationMessage) {
            taskCh <- notif
        }),
        claudeagent.WithAgents(map[string]claudeagent.AgentDefinition{
            "analyzer": {
                Description: "Analyzes data",
                Prompt:      "You analyze things quickly",
            },
        }),
    )
    require.NoError(t, err)
    defer client.Close()

    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    for msg := range client.Query(ctx, "Run the analyzer in background") {
        if _, ok := msg.(claudeagent.ResultMessage); ok {
            break
        }
    }

    select {
    case notif := <-taskCh:
        assert.NotEmpty(t, notif.TaskID)
    case <-time.After(30 * time.Second):
        t.Fatal("timeout waiting for task notification")
    }
}
```

---

## Implementation Order

1. **Phase 1: Message Types** - Core message parsing (low risk, foundation)
2. **Phase 2: Tool Input Types** - Type definitions (low risk)
   - Updated for v0.2.30: `AgentInput` now includes `name`, `team_name`, `mode` fields
3. **Phase 3: Agent Definition** - Expand existing type (low risk)
4. **Phase 4: High-Level API** - New file with task management (medium risk)
5. **Phase 5: Client Integration** - Wire up notifications (medium risk)
6. **Phase 6: Hook Enhancement** - Minor additions (low risk)
   - Updated for v0.2.30: New `PermissionRequest` and `Setup` hook events

Each phase can be implemented and tested independently.

---

## Open Questions

1. **Environment Variable**: The TypeScript SDK has `CLAUDE_CODE_ENABLE_TASKS`. Should we expose this as a client option or assume it's always enabled in newer CLI versions?

2. **Task Output Reading**: The `output_file` in task notifications points to a file. Should the SDK provide helpers to read this, or leave it to the user?

3. **Task Cancellation**: The TypeScript SDK doesn't expose task cancellation. Should we add a `Task.Cancel()` method that sends an interrupt?

4. **Agent ID Generation**: When spawning agents programmatically, who generates the agent ID - the SDK or the CLI?

5. **Permission Mode for Subagents**: The v0.2.30 `mode` field on `AgentInput` allows controlling subagent permissions. Should we expose this as a Go option on the agent definition or only surface it in tool input parsing?

6. **Named Agents**: The v0.2.30 `name` and `team_name` fields enable agent identification. Should the Go SDK track agents by name in the `TaskManager`?

---

## Compatibility Notes

- Requires Claude Code CLI v2.1.30+ for full task system support
- The `CLAUDE_CODE_ENABLE_TASKS` environment variable may need to be set for older CLI versions
- Backward compatible - existing code that doesn't use tasks continues to work
- The `name`, `team_name`, and `mode` fields on `AgentInput` are optional and backward-compatible with older CLI versions
- The `TodoWrite` tool is still available but `TaskCreate`/`TaskUpdate`/`TaskList` are preferred for structured task management

## v0.2.30 Changelog (Task-Related)

Key changes from v0.2.17 to v0.2.30 affecting the task system:

1. **AgentInput**: Added `name` (agent label), `team_name` (team context), and `mode` (permission mode) fields
2. **PermissionMode**: Full enum type with 6 values: `default`, `acceptEdits`, `bypassPermissions`, `plan`, `delegate`, `dontAsk`
3. **New Hook Events**: `PermissionRequest` (tool permission interception), `Setup` (session initialization)
4. **New SDK Options**: `agent` (run as agent type), `enableFileCheckpointing`, `forkSession`, `persistSession`, `maxBudgetUsd`, `fallbackModel`, `betas`, `tools` (tool restriction)
5. **New Message Types**: `SDKToolProgressMessage`, `SDKToolUseSummaryMessage`, `SDKFilesPersistedEvent`, `SDKAuthStatusMessage`, `SDKHookStartedMessage`, `SDKHookProgressMessage`, `SDKHookResponseMessage`, `SDKCompactBoundaryMessage`
6. **ConfigInput Tool**: New tool for getting/setting Claude Code configuration
7. **ExitPlanModeInput**: Added remote session push capabilities (`pushToRemote`, `remoteSessionId`, `remoteSessionUrl`, `remoteSessionTitle`)
8. **TodoWrite Coexistence**: The legacy `TodoWrite` tool (flat `{ content, status, activeForm }[]`) coexists alongside the structured `TaskCreate`/`TaskUpdate`/`TaskList` tools
