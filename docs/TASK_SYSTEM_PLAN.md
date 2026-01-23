# Task System Implementation Plan

This document outlines the plan for exposing the new Task system in the Go SDK, based on analysis of the TypeScript SDK's implementation (version 0.2.17).

## Executive Summary

The Task system enables Claude to spawn specialized subagents for delegating work. Key capabilities include:

1. **Programmatic Agent Definitions** - Define custom subagents with specific tools, prompts, and models
2. **Background Task Execution** - Run tasks asynchronously with status notifications
3. **Task Output Retrieval** - Poll or block for background task results
4. **Agent Lifecycle Hooks** - Intercept subagent start/stop events
5. **Shared Task Lists** - Multiple instances share TodoWrite storage via `CLAUDE_CODE_TASK_LIST_ID`

The Go SDK already has partial support (agent definitions, subagent hooks), but lacks:
- `TaskNotificationMessage` for background task completion
- `TaskOutput` tool input types for retrieving results
- High-level APIs for task management
- Task list sharing via environment variables

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

// Task tool input (Agent/Task tool)
interface AgentInput {
  description: string;              // Short task description (3-5 words)
  prompt: string;                   // Task for agent to perform
  subagent_type: string;            // Agent type to use
  model?: 'sonnet' | 'opus' | 'haiku';
  resume?: string;                  // Agent ID to resume
  run_in_background?: boolean;      // Async execution
  max_turns?: number;               // Turn limit
  name?: string;                    // Agent name
  team_name?: string;               // Team context
  mode?: PermissionMode;            // Permission mode
}

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
// TaskInput represents the input for the Task tool.
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

    // Name labels the spawned agent for tracking.
    Name string `json:"name,omitempty"`

    // TeamName specifies team context for the task.
    TeamName string `json:"team_name,omitempty"`

    // Mode sets the permission mode for the subagent.
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

### Core (Required)
1. **Phase 1: Message Types** - Core message parsing (low risk, foundation)
2. **Phase 2: Tool Input Types** - Task/TaskOutput input definitions (low risk)
3. **Phase 3: Agent Definition** - Expand existing type (low risk)
4. **Phase 4: Background Task API** - TaskManager for subagent notifications (medium risk)
5. **Phase 5: Client Integration** - Wire up notifications (medium risk)
6. **Phase 6: Hook Enhancement** - Minor additions (low risk)

### Task List System (New)
7. **Phase 7: Task CRUD Types** - TaskCreate/Update/Get/List input types (low risk)
8. **Phase 8: TaskStore Interface** - Pluggable storage backend interface (medium risk)
9. **Phase 9: FileTaskStore** - Default JSON file implementation (medium risk)
10. **Phase 10: TaskManager API** - High-level task management (medium risk)
11. **Phase 11: Client Task Integration** - `WithTaskListID()`, `WithTaskStore()` (low risk)

### Optional Extensions
12. **Phase 12: PostgresTaskStore** - PostgreSQL backend (optional)
13. **Phase 13: Task Subscriptions** - Real-time task change notifications (optional)

Each phase can be implemented and tested independently.

---

## Open Questions

### Background Tasks (Subagents)

1. **Environment Variable**: The TypeScript SDK has `CLAUDE_CODE_ENABLE_TASKS`. Should we expose this as a client option or assume it's always enabled in newer CLI versions?

2. **Task Output Reading**: The `output_file` in task notifications points to a file. Should the SDK provide helpers to read this, or leave it to the user?

3. **Task Cancellation**: The TypeScript SDK doesn't expose task cancellation. Should we add a `Task.Cancel()` method that sends an interrupt?

4. **Agent ID Generation**: When spawning agents programmatically, who generates the agent ID - the SDK or the CLI?

### Task List System

5. **Naming Clarity**: Two different "task" systems exist:
   - **Background Tasks** (`run_in_background`): Subagent execution with notifications
   - **Task List** (`CLAUDE_CODE_TASK_LIST_ID`): Persistent CRUD storage

   Should we rename one to avoid confusion? e.g., "WorkItem" vs "Task"?

6. **Custom Store Sync Strategy**: When using a custom TaskStore (e.g., Postgres):
   - **Option A**: Sync to JSON files so CLI can read them (simple but duplicated data)
   - **Option B**: MCP proxy that intercepts Task* tools (elegant but complex)
   - **Option C**: Require users to use only SDK for task management (limiting)

   Which approach is best?

7. **Task Blocking Logic**: The CLI auto-unblocks tasks when blockers complete. Should:
   - The SDK replicate this logic in custom stores?
   - Or rely on the CLI to handle blocking/unblocking?

8. **Concurrent Access**: Multiple Claude instances may modify the same task list.
   - FileTaskStore uses file locking (see `proper-lockfile` in CLI)
   - Should TaskStore interface require atomic operations?

9. **Task IDs**: CLI uses incrementing integers (`1`, `2`, `3`). Should custom stores:
   - Use the same scheme?
   - Allow UUIDs or other ID formats?
   - Does the CLI care about ID format?

---

## Shared Task Lists (CLAUDE_CODE_TASK_LIST_ID)

The CLI supports a **shared task list** feature via `CLAUDE_CODE_TASK_LIST_ID`. This is separate from background tasks (subagents) - it's the persistent storage for the `TodoWrite` tool.

### How It Works

Tasks created via `TodoWrite` are stored at:
```
~/.claude/tasks/{taskListId}/{taskId}.json
```

The task list ID is determined by (in order of precedence):
1. `CLAUDE_CODE_TASK_LIST_ID` environment variable
2. Session ID
3. Generated unique ID

### Use Case: Multi-Instance Coordination

Multiple Claude Code instances can share the same task list:

```go
// Instance 1: Team leader creates tasks
client1, _ := claudeagent.NewClient(
    claudeagent.WithEnv(map[string]string{
        "CLAUDE_CODE_TASK_LIST_ID": "project-alpha-tasks",
    }),
)

// Instance 2: Worker picks up tasks
client2, _ := claudeagent.NewClient(
    claudeagent.WithEnv(map[string]string{
        "CLAUDE_CODE_TASK_LIST_ID": "project-alpha-tasks",
    }),
)
```

### Task CRUD Tools

Claude has four tools for task management:

#### TaskCreate
Creates a new task with metadata:
```json
{
  "tool": "TaskCreate",
  "subject": "Set up database connection",
  "description": "Configure PostgreSQL connection pool, create users table",
  "activeForm": "Setting up database connection",
  "metadata": {
    "priority": "high",
    "estimate": "30min"
  }
}
```
Tasks start with `status: "pending"` and no owner.

#### TaskUpdate
Modify any aspect of an existing task:
```json
{
  "tool": "TaskUpdate",
  "taskId": "3",
  "status": "in_progress",
  "owner": "backend-dev",
  "addBlockedBy": ["1", "2"]
}
```
Note: `addBlocks` and `addBlockedBy` **append** to arrays - they don't replace them.
Blocked tasks can only become unblocked when blocking tasks are marked `completed`.

#### TaskGet
Retrieve full details of a specific task:
```json
{
  "tool": "TaskGet",
  "taskId": "3"
}
```

#### TaskList
See all tasks at once:
```json
{
  "tool": "TaskList"
}
```

### Task List Schema

Each task in the list has this structure:

```go
// TaskListItem represents a persistent task in the shared task list.
type TaskListItem struct {
    ID          string            `json:"id"`
    Subject     string            `json:"subject"`
    Description string            `json:"description"`
    ActiveForm  string            `json:"activeForm,omitempty"`
    Owner       string            `json:"owner,omitempty"`      // Agent ID that owns this task
    Status      TaskListStatus    `json:"status"`               // pending, in_progress, completed
    Blocks      []string          `json:"blocks"`               // Task IDs this blocks
    BlockedBy   []string          `json:"blockedBy"`            // Task IDs blocking this
    Metadata    map[string]any    `json:"metadata,omitempty"`
}

type TaskListStatus string

const (
    TaskListStatusPending    TaskListStatus = "pending"
    TaskListStatusInProgress TaskListStatus = "in_progress"
    TaskListStatusCompleted  TaskListStatus = "completed"
)
```

### Tool Input Types for Go SDK

```go
// TaskCreateInput is the input for the TaskCreate tool.
type TaskCreateInput struct {
    Subject     string         `json:"subject"`
    Description string         `json:"description"`
    ActiveForm  string         `json:"activeForm,omitempty"`
    Metadata    map[string]any `json:"metadata,omitempty"`
}

// TaskUpdateInput is the input for the TaskUpdate tool.
type TaskUpdateInput struct {
    TaskID       string         `json:"taskId"`
    Subject      string         `json:"subject,omitempty"`
    Description  string         `json:"description,omitempty"`
    ActiveForm   string         `json:"activeForm,omitempty"`
    Status       TaskListStatus `json:"status,omitempty"`
    Owner        string         `json:"owner,omitempty"`
    AddBlocks    []string       `json:"addBlocks,omitempty"`    // Appends to blocks
    AddBlockedBy []string       `json:"addBlockedBy,omitempty"` // Appends to blockedBy
    Metadata     map[string]any `json:"metadata,omitempty"`
}

// TaskGetInput is the input for the TaskGet tool.
type TaskGetInput struct {
    TaskID string `json:"taskId"`
}

// TaskListInput is the input for the TaskList tool.
// Currently has no parameters but defined for consistency.
type TaskListInput struct{}
```

### Pluggable Task Store Interface

The default storage is on-disk JSON at `~/.claude/tasks/{listId}/{taskId}.json`.
We can provide an interface for custom backends (PostgreSQL, Redis, etc.):

```go
// TaskStore is the interface for task persistence backends.
//
// Implementations can store tasks in various backends:
// - FileTaskStore (default): JSON files on disk
// - PostgresTaskStore: PostgreSQL database
// - RedisTaskStore: Redis key-value store
// - MemoryTaskStore: In-memory for testing
type TaskStore interface {
    // Create adds a new task and returns its ID.
    Create(ctx context.Context, listID string, task TaskListItem) (string, error)

    // Get retrieves a task by ID.
    Get(ctx context.Context, listID, taskID string) (*TaskListItem, error)

    // Update modifies an existing task.
    Update(ctx context.Context, listID, taskID string, update TaskUpdateInput) error

    // List returns all tasks in a list.
    List(ctx context.Context, listID string) ([]TaskListItem, error)

    // Delete removes a task.
    Delete(ctx context.Context, listID, taskID string) error

    // Subscribe returns a channel for task change notifications.
    // Returns nil if the backend doesn't support subscriptions.
    Subscribe(ctx context.Context, listID string) (<-chan TaskEvent, error)
}

// TaskEvent represents a change to a task.
type TaskEvent struct {
    Type   TaskEventType
    TaskID string
    Task   *TaskListItem // nil for delete events
}

type TaskEventType string

const (
    TaskEventCreated  TaskEventType = "created"
    TaskEventUpdated  TaskEventType = "updated"
    TaskEventDeleted  TaskEventType = "deleted"
)
```

### SDK Task Management API

Direct task management from Go code:

```go
// TaskManager provides programmatic access to the task list.
type TaskManager struct {
    store  TaskStore
    listID string
}

// NewTaskManager creates a task manager with the default file store.
func NewTaskManager(listID string) *TaskManager {
    return &TaskManager{
        store:  NewFileTaskStore(),
        listID: listID,
    }
}

// NewTaskManagerWithStore creates a task manager with a custom store.
func NewTaskManagerWithStore(listID string, store TaskStore) *TaskManager {
    return &TaskManager{
        store:  store,
        listID: listID,
    }
}

// Create adds a new task.
func (tm *TaskManager) Create(ctx context.Context, subject, description string, opts ...TaskOption) (*TaskListItem, error)

// Get retrieves a task by ID.
func (tm *TaskManager) Get(ctx context.Context, taskID string) (*TaskListItem, error)

// Update modifies a task.
func (tm *TaskManager) Update(ctx context.Context, taskID string, opts ...TaskUpdateOption) error

// List returns all tasks.
func (tm *TaskManager) List(ctx context.Context) ([]TaskListItem, error)

// ListPending returns tasks with status "pending".
func (tm *TaskManager) ListPending(ctx context.Context) ([]TaskListItem, error)

// ListByOwner returns tasks owned by a specific agent.
func (tm *TaskManager) ListByOwner(ctx context.Context, ownerID string) ([]TaskListItem, error)

// ListUnblocked returns pending tasks that have no blockers.
func (tm *TaskManager) ListUnblocked(ctx context.Context) ([]TaskListItem, error)

// Claim assigns an owner to a pending task and sets it to in_progress.
func (tm *TaskManager) Claim(ctx context.Context, taskID, ownerID string) error

// Complete marks a task as completed (auto-unblocks dependent tasks).
func (tm *TaskManager) Complete(ctx context.Context, taskID string) error

// Watch returns a channel for task updates (if store supports it).
func (tm *TaskManager) Watch(ctx context.Context) (<-chan TaskEvent, error)
```

### Example: PostgreSQL Task Store

```go
// PostgresTaskStore implements TaskStore using PostgreSQL.
type PostgresTaskStore struct {
    db *sql.DB
}

func NewPostgresTaskStore(connString string) (*PostgresTaskStore, error) {
    db, err := sql.Open("postgres", connString)
    if err != nil {
        return nil, err
    }
    return &PostgresTaskStore{db: db}, nil
}

// Usage with SDK
func main() {
    pgStore, _ := NewPostgresTaskStore("postgres://localhost/tasks")

    taskManager := claudeagent.NewTaskManagerWithStore("project-alpha", pgStore)

    client, _ := claudeagent.NewClient(
        claudeagent.WithTaskListID("project-alpha"),
        claudeagent.WithTaskStore(pgStore), // SDK uses same store
    )

    // Pre-populate tasks from external system
    taskManager.Create(ctx, "Build auth module", "Implement OAuth2 flow",
        claudeagent.WithTaskMetadata(map[string]any{"jira": "AUTH-123"}),
    )

    // Claude can now see and work on these tasks
    for msg := range client.Query(ctx, "Check the task list and start working") {
        // ...
    }
}
```

### API Design for Go SDK Options

```go
// WithTaskListID sets the shared task list ID.
// Multiple instances with the same ID share the same task list.
func WithTaskListID(id string) Option {
    return func(o *Options) {
        if o.Env == nil {
            o.Env = make(map[string]string)
        }
        o.Env["CLAUDE_CODE_TASK_LIST_ID"] = id
    }
}

// WithTaskStore sets a custom task storage backend.
// The SDK will use this store for task operations instead of the default file store.
func WithTaskStore(store TaskStore) Option {
    return func(o *Options) {
        o.TaskStore = store
    }
}

// Convenience method combining common env vars
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
```

### Architecture: SDK ↔ CLI Task Sync

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Go Application                               │
├─────────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐     ┌─────────────────┐     ┌──────────────────┐  │
│  │   Client    │────▶│  TaskManager    │────▶│   TaskStore      │  │
│  │             │     │                 │     │   (interface)    │  │
│  │ Query()     │     │ Create/Update   │     ├──────────────────┤  │
│  │ Stream()    │     │ List/Claim      │     │ FileTaskStore    │  │
│  └──────┬──────┘     └────────┬────────┘     │ PostgresStore    │  │
│         │                     │              │ RedisStore       │  │
│         ▼                     ▼              └────────┬─────────┘  │
│  ┌─────────────────────────────────────────────────────┴──────┐    │
│  │              Shared Storage (synchronized)                  │    │
│  │     ~/.claude/tasks/{listId}/ OR custom backend            │    │
│  └─────────────────────────────────────────────────────────────┘    │
│         ▲                     ▲                                     │
│         │                     │                                     │
├─────────┼─────────────────────┼─────────────────────────────────────┤
│         │    subprocess       │                                     │
│         │    stdin/stdout     │                                     │
│         ▼                     ▼                                     │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Claude Code CLI                           │   │
│  │  ┌────────────┐  ┌────────────┐  ┌────────────┐             │   │
│  │  │ TaskCreate │  │ TaskUpdate │  │  TaskList  │             │   │
│  │  │ TaskGet    │  │   ...      │  │            │             │   │
│  │  └────────────┘  └────────────┘  └────────────┘             │   │
│  │         │              │               │                     │   │
│  │         ▼              ▼               ▼                     │   │
│  │  ┌─────────────────────────────────────────────────────┐    │   │
│  │  │          CLI Task Storage (reads/writes JSON)        │    │   │
│  │  │          ~/.claude/tasks/{listId}/                   │    │   │
│  │  └─────────────────────────────────────────────────────┘    │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

**Key insight**: When using a custom TaskStore (e.g., PostgreSQL), the SDK and CLI
need to stay synchronized. Two approaches:

1. **Sync on demand**: SDK writes to custom store, syncs to JSON files that CLI reads
2. **MCP proxy**: SDK provides an MCP server that proxies Task* tools to custom store

The MCP proxy approach is more elegant and fits the existing architecture.

### Related Environment Variables

| Variable | Purpose |
|----------|---------|
| `CLAUDE_CODE_TASK_LIST_ID` | Shared task list identifier |
| `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS` | Disable background task execution |
| `CLAUDE_CODE_AGENT_ID` | Override the agent ID |
| `CLAUDE_CODE_TEAM_NAME` | Team context for task assignment |

---

## Compatibility Notes

- Requires Claude Code CLI version with task system support
- The `CLAUDE_CODE_ENABLE_TASKS` environment variable may need to be set for background tasks
- `CLAUDE_CODE_TASK_LIST_ID` enables shared TodoWrite storage
- Backward compatible - existing code that doesn't use tasks continues to work
