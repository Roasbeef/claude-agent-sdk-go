# Claude Code Task Tools

This document describes the task management tools available to Claude Code during conversations. These tools enable Claude to track progress, organize multi-step work, and coordinate with other agents.

## Overview

Claude Code has access to four task tools that persist tasks to `~/.claude/tasks/{listID}/`:

| Tool | Purpose |
|------|---------|
| `TaskCreate` | Create a new task with subject, description, and metadata |
| `TaskGet` | Retrieve full details of a specific task by ID |
| `TaskUpdate` | Update task status, add dependencies, or modify fields |
| `TaskList` | List all tasks with summary information |

## Tool Reference

### TaskCreate

Creates a new task and returns its auto-generated ID.

**Parameters:**
- `subject` (required): Brief title in imperative form (e.g., "Fix authentication bug")
- `description` (required): Detailed description of what needs to be done
- `activeForm` (optional): Present continuous form shown in spinner (e.g., "Fixing authentication bug")
- `metadata` (optional): Arbitrary key-value metadata (priority, size, tags, etc.)

**Example:**
```json
{
  "subject": "Implement user login",
  "description": "Add OAuth2 flow with Google and GitHub providers",
  "activeForm": "Implementing user login",
  "metadata": {
    "priority": "P1",
    "size": "M",
    "tags": ["auth", "backend"]
  }
}
```

**Returns:** Task ID (e.g., "1", "2", etc.)

### TaskGet

Retrieves full task details including description and dependencies.

**Parameters:**
- `taskId` (required): The ID of the task to retrieve

**Returns:**
- `id`: Task identifier
- `subject`: Task title
- `description`: Full requirements and context
- `status`: 'pending', 'in_progress', or 'completed'
- `owner`: Agent ID if assigned
- `blocks`: Task IDs this task blocks
- `blockedBy`: Task IDs blocking this task
- `metadata`: Custom metadata

### TaskUpdate

Updates an existing task. Only specified fields are modified.

**Parameters:**
- `taskId` (required): The ID of the task to update
- `status` (optional): New status ('pending', 'in_progress', 'completed', or 'deleted')
- `subject` (optional): New subject
- `description` (optional): New description
- `activeForm` (optional): New activeForm
- `owner` (optional): Agent name to assign
- `addBlocks` (optional): Task IDs that this task blocks
- `addBlockedBy` (optional): Task IDs that block this task
- `metadata` (optional): Metadata keys to merge (set value to null to delete key)

**Status Values:**
- `pending`: Task is waiting to be started
- `in_progress`: Task is actively being worked on
- `completed`: Task has been finished
- `deleted`: Permanently removes the task

### TaskList

Lists all tasks with summary information.

**Parameters:** None

**Returns:** Array of task summaries:
- `id`: Task identifier
- `subject`: Brief description
- `status`: Current status
- `owner`: Assigned agent (if any)
- `blockedBy`: List of blocking task IDs

## Task Lifecycle

```
┌──────────┐      TaskUpdate        ┌─────────────┐      TaskUpdate       ┌───────────┐
│ pending  │ ──────────────────────►│ in_progress │ ─────────────────────►│ completed │
└──────────┘   status: in_progress  └─────────────┘   status: completed   └───────────┘
     │                                     │
     │              TaskUpdate             │
     └─────────────────────────────────────┘
              status: deleted (removes task)
```

**Best Practice:** Always set `in_progress` before starting work, `completed` when done.

## Task Dependencies

Tasks can block other tasks to enforce ordering:

```json
// Task 2 cannot start until Task 1 completes
{
  "taskId": "2",
  "addBlockedBy": ["1"]
}
```

When a blocking task is completed, dependent tasks are automatically unblocked.

## When to Use Task Tools

Use task tools when:
- Working on multi-step tasks (3+ distinct steps)
- Complex tasks requiring careful tracking
- User provides multiple tasks to complete
- Coordination between multiple agents is needed

Skip task tools when:
- Single, trivial task
- Task can be completed in less than 3 simple steps
- Purely conversational or informational requests

## Hooks and Integration

### Environment Variable

The `CLAUDE_CODE_TASK_LIST_ID` environment variable specifies which task list to use. When set, both the CLI tools and SDK share the same task list.

```bash
export CLAUDE_CODE_TASK_LIST_ID="my-project"
```

### SDK Integration

The Go SDK can create a `TaskManager` that shares the same task list:

```go
client, _ := claudeagent.NewClient(
    claudeagent.WithTaskListID("my-project"),
)
tm, _ := client.TaskManager()
```

See [TASKS.md](TASKS.md) for the full SDK API documentation.

### Hook Availability

Task tools are available in all Claude Code contexts:
- Interactive CLI sessions
- SDK-spawned Claude instances
- Subagents spawned via the Task tool
- GitHub Actions workflows (via claude-code-action)

## Storage

Tasks are persisted as JSON files at:
```
~/.claude/tasks/{listID}/{taskID}.json
```

Each task file contains:
```json
{
  "id": "1",
  "subject": "Implement feature X",
  "description": "Full description...",
  "status": "pending",
  "owner": "",
  "blocks": [],
  "blockedBy": [],
  "activeForm": "Implementing feature X",
  "metadata": {
    "priority": "P1"
  }
}
```

## Platform Support

### FileTaskStore (Unix/Linux/macOS)

File-based persistence with flock-based locking. This is the default on Unix-like systems.

### MemoryTaskStore (All Platforms)

In-memory storage for testing. On Windows, `NewFileTaskStore()` returns `ErrFileTaskStoreNotSupported`, so use `NewMemoryTaskStore()` instead.

## Multi-Agent Coordination

Multiple Claude instances can coordinate via shared task lists:

1. **Agent A** creates tasks and sets dependencies
2. **Agent B** calls `TaskList` to find available work
3. **Agent B** updates task to `in_progress` with their owner ID
4. **Agent B** completes work and marks task `completed`
5. Blocked tasks are automatically unblocked

### Claiming Tasks

To prevent conflicts, agents should "claim" tasks by setting the owner:

```json
{
  "taskId": "1",
  "status": "in_progress",
  "owner": "agent-backend"
}
```

### Watching for Work

Agents can poll `TaskList` to find unclaimed, unblocked tasks:

```
For each task in TaskList:
  If status == "pending" AND owner == "" AND blockedBy == []:
    This task is available to claim
```

## Example Workflow

```
User: "Implement user authentication with tests"

Claude:
1. TaskCreate: "Design auth architecture" (description: "Plan OAuth flow...")
2. TaskCreate: "Implement auth service" (blockedBy: ["1"])
3. TaskCreate: "Write auth tests" (blockedBy: ["2"])

4. TaskUpdate: task 1 -> in_progress
5. [Does architecture work]
6. TaskUpdate: task 1 -> completed

7. TaskUpdate: task 2 -> in_progress  (now unblocked)
8. [Implements auth service]
9. TaskUpdate: task 2 -> completed

10. TaskUpdate: task 3 -> in_progress (now unblocked)
11. [Writes tests]
12. TaskUpdate: task 3 -> completed
```

## Metadata Conventions

Common metadata fields used by convention:

| Field | Values | Description |
|-------|--------|-------------|
| `priority` | P0, P1, P2, P3 | Task urgency (P0 = critical) |
| `size` | XS, S, M, L, XL | Estimated effort |
| `tags` | string[] | Categorization labels |
| `shortname` | string | Brief identifier |
| `acceptance_criteria` | string | Definition of done |
| `blocked_reason` | string | Why task is blocked (beyond dependencies) |

## Error Handling

- `ErrTaskNotFound`: Returned when task ID doesn't exist
- `ErrFileTaskStoreNotSupported`: Returned on Windows (use MemoryTaskStore)

## Related Documentation

- [TASKS.md](TASKS.md) - Go SDK TaskManager API
- [cli-protocol.md](cli-protocol.md) - Claude Code CLI protocol
- [DESIGN.md](DESIGN.md) - Overall SDK architecture
