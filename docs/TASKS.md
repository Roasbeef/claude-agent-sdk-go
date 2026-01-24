# Task List System

The Task List system enables multiple Claude Code instances and SDK applications to share persistent task lists for multi-agent coordination. Tasks persist at `~/.claude/tasks/{listID}/` and can be shared via the `CLAUDE_CODE_TASK_LIST_ID` environment variable.

## Quick Start

```go
// Create a client with a shared task list
client, _ := claudeagent.NewClient(
    claudeagent.WithTaskListID("my-project"),
)
defer client.Close()

// Get the TaskManager
tm, _ := client.TaskManager()
ctx := context.Background()

// Create a task
task, _ := tm.Create(ctx, "Build auth module", "Implement OAuth2 flow",
    claudeagent.WithPriority("high"),
    claudeagent.WithEstimate("4h"),
)

// Work with the task
tm.Claim(ctx, task.ID, "backend-agent")
// ... do work ...
tm.Complete(ctx, task.ID)
```

## Architecture

The task system consists of four layers:

```
┌─────────────────────────────────────────────────────┐
│                    TaskManager                       │
│         High-level API with functional options       │
├─────────────────────────────────────────────────────┤
│                    TaskStore                         │
│              Interface for persistence               │
├──────────────────────┬──────────────────────────────┤
│    FileTaskStore     │      MemoryTaskStore         │
│   ~/.claude/tasks/   │      (for testing)           │
└──────────────────────┴──────────────────────────────┘
```

### Core Types

| Type | Description |
|------|-------------|
| `TaskListItem` | The persistent task struct with subject, description, status, owner, and metadata |
| `TaskListStatus` | Lifecycle states: `pending`, `in_progress`, `completed` |
| `TaskEvent` | Event notifications for subscriptions |
| `TaskStore` | Interface for task persistence backends |

## Task Lifecycle

```
                    ┌──────────┐
                    │ pending  │
                    └────┬─────┘
                         │ Claim()
                         ▼
                  ┌─────────────┐
                  │ in_progress │
                  └──────┬──────┘
                         │ Complete()
                         ▼
                   ┌───────────┐
                   │ completed │
                   └───────────┘
```

## Creating Tasks

Tasks are created with a subject and description. Optional functional options allow setting metadata:

```go
task, err := tm.Create(ctx, "Subject", "Description",
    claudeagent.WithActiveForm("Working on subject"),  // Shown in spinners
    claudeagent.WithPriority("high"),                   // Metadata
    claudeagent.WithEstimate("2h"),                     // Metadata
    claudeagent.WithMetadata(map[string]any{            // Custom metadata
        "tags": []string{"backend", "auth"},
    }),
)
```

## Updating Tasks

Updates use functional options to modify specific fields:

```go
err := tm.Update(ctx, task.ID,
    claudeagent.UpdateSubject("New subject"),
    claudeagent.UpdateStatus(claudeagent.TaskListStatusInProgress),
    claudeagent.UpdateOwner("agent-id"),
    claudeagent.AddBlockedBy("other-task-id"),
    claudeagent.UpdateMetadata(map[string]any{"priority": "low"}),
)
```

## Task Dependencies

Tasks can block other tasks. Blocked tasks won't appear in `ListUnblocked()` until their blockers complete:

```go
// Create prerequisite and dependent tasks
prereq, _ := tm.Create(ctx, "Set up database", "Configure PostgreSQL")
feature, _ := tm.Create(ctx, "Build user API", "CRUD endpoints")

// Mark feature as blocked by prereq
tm.Update(ctx, feature.ID, claudeagent.AddBlockedBy(prereq.ID))

// Only prereq is available
available, _ := tm.ListUnblocked(ctx)  // Returns [prereq]

// Completing prereq automatically unblocks feature
tm.Complete(ctx, prereq.ID)
available, _ = tm.ListUnblocked(ctx)   // Now returns [feature]
```

## Querying Tasks

```go
// List all tasks
tasks, _ := tm.List(ctx)

// Filter by status
pending, _ := tm.ListPending(ctx)
inProgress, _ := tm.ListInProgress(ctx)

// Filter by owner
myTasks, _ := tm.ListByOwner(ctx, "my-agent-id")

// Get available tasks (pending, unclaimed, unblocked)
available, _ := tm.ListUnblocked(ctx)

// Get next available task
next, _ := tm.NextAvailable(ctx)

// Claim the next available task
claimed, _ := tm.ClaimNext(ctx, "my-agent-id")

// Get statistics
stats, _ := tm.Stats(ctx)
// stats.Total, stats.Pending, stats.InProgress, stats.Completed, stats.Blocked
```

## Event Subscriptions

Subscribe to real-time task events:

```go
eventCh, _ := tm.Watch(ctx)

go func() {
    for event := range eventCh {
        switch event.Type {
        case claudeagent.TaskEventCreated:
            fmt.Printf("Created: %s\n", event.Task.Subject)
        case claudeagent.TaskEventClaimed:
            fmt.Printf("Claimed by: %s\n", event.Task.Owner)
        case claudeagent.TaskEventCompleted:
            fmt.Printf("Completed: %s\n", event.TaskID)
        case claudeagent.TaskEventUnblocked:
            fmt.Printf("Unblocked: %s\n", event.Task.Subject)
        case claudeagent.TaskEventUpdated:
            fmt.Printf("Updated: %s\n", event.TaskID)
        case claudeagent.TaskEventDeleted:
            fmt.Printf("Deleted: %s\n", event.TaskID)
        }
    }
}()
```

## Storage Backends

### FileTaskStore (Default)

Persists tasks as JSON files at `~/.claude/tasks/{listID}/{taskID}.json`. This is the default store used by `TaskManager` and is compatible with the Claude Code CLI.

```go
// Use default path (~/.claude/tasks/)
store, _ := claudeagent.NewFileTaskStore("")

// Use custom path
store, _ := claudeagent.NewFileTaskStore("/custom/path")
```

Features:
- Atomic writes (temp file + rename)
- File locking via `Lock()` and `TryLock()`
- Auto-incrementing IDs
- Export/Import for backup and migration

### MemoryTaskStore

In-memory store for testing:

```go
store := claudeagent.NewMemoryTaskStore()
tm := claudeagent.NewTaskManagerWithStore("test-list", store)
```

### Custom Stores

Implement the `TaskStore` interface for custom backends:

```go
type TaskStore interface {
    Create(ctx context.Context, listID string, task TaskListItem) (string, error)
    Get(ctx context.Context, listID, taskID string) (*TaskListItem, error)
    Update(ctx context.Context, listID, taskID string, update TaskUpdateInput) error
    List(ctx context.Context, listID string) ([]TaskListItem, error)
    Delete(ctx context.Context, listID, taskID string) error
    Clear(ctx context.Context, listID string) error
    Subscribe(ctx context.Context, listID string) (<-chan TaskEvent, error)
}
```

Optional extended interfaces:
- `TaskStoreWithLocking`: Distributed locking for concurrent access
- `TaskStoreWithExport`: Bulk export/import operations

## Multi-Agent Coordination

Multiple agents can share a task list for coordinated work:

```go
// Agent 1: Create tasks
tm1, _ := claudeagent.NewTaskManager("shared-project")
task, _ := tm1.Create(ctx, "Build feature X", "Implement the feature")

// Agent 2: Claim and work on tasks
tm2, _ := claudeagent.NewTaskManager("shared-project")
available, _ := tm2.ListUnblocked(ctx)
if len(available) > 0 {
    tm2.Claim(ctx, available[0].ID, "agent-2")
    // ... do work ...
    tm2.Complete(ctx, available[0].ID)
}
```

## CLI Integration

When using `WithTaskListID()`, the `CLAUDE_CODE_TASK_LIST_ID` environment variable is automatically passed to the CLI subprocess, enabling the CLI's task tools (`TaskCreate`, `TaskList`, `TaskUpdate`, `TaskGet`) to use the same task list:

```go
client, _ := claudeagent.NewClient(
    claudeagent.WithTaskListID("my-project"),
)

// Both SDK and CLI now share the same task list
tm, _ := client.TaskManager()
tm.Create(ctx, "SDK task", "Created by SDK")

// Claude CLI will see this task via TaskList tool
```

## Best Practices

1. **Use meaningful subjects**: Write subjects in imperative form ("Build auth module" not "Auth module")
2. **Set activeForm**: Provide present continuous form for spinner display ("Building auth module")
3. **Use dependencies wisely**: Create blocking relationships to enforce task ordering
4. **Claim before working**: Always claim a task before starting work to prevent conflicts
5. **Check IsAvailable()**: Use `ListUnblocked()` to find tasks that are ready to work on
6. **Handle errors**: Check for `ErrTaskNotFound` when tasks may have been deleted
