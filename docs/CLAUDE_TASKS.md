# Claude Code Task Tools

> Based on Claude Code CLI v2.1.30 / SDK v0.2.30

This document describes the task management tools available to Claude Code during conversations. These tools enable Claude to track progress, organize multi-step work, and coordinate with other agents.

## Overview

Claude Code provides two categories of task-related tools:

### Task Management Tools

These tools persist tasks to `~/.claude/tasks/{listID}/` for tracking work items:

| Tool | Purpose |
|------|---------|
| `TaskCreate` | Create a new task with subject, description, and metadata |
| `TaskGet` | Retrieve full details of a specific task by ID |
| `TaskUpdate` | Update task status, add dependencies, or modify fields |
| `TaskList` | List all tasks with summary information |

### Background Task Tools

These tools manage background agent/subprocess execution:

| Tool | Purpose |
|------|---------|
| `Task` (Agent) | Spawn a subagent to perform delegated work |
| `TaskOutput` | Retrieve output from a running or completed background task |
| `TaskStop` | Stop a running background task |

### Legacy Tool

| Tool | Purpose |
|------|---------|
| `TodoWrite` | Simple flat todo list (no IDs, dependencies, or metadata). Superseded by `TaskCreate`/`TaskUpdate`/`TaskList` for new usage. |

## Task Management Tool Reference

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

## Background Task Tool Reference

### Task (Agent Spawning)

Spawns a specialized subagent to perform delegated work. The agent type determines which agent definition to use.

**Parameters:**
- `description` (required): Short (3-5 word) summary of the task
- `prompt` (required): Detailed task for the agent to perform
- `subagent_type` (required): Which agent definition to use
- `model` (optional): Model override - `"sonnet"`, `"opus"`, or `"haiku"`. Inherits from parent if omitted.
- `resume` (optional): Agent ID to resume from a previous execution
- `run_in_background` (optional): Set `true` for async execution. Returns immediately with `task_id` and `output_file`.
- `max_turns` (optional): Maximum API round-trips before stopping
- `name` (optional): Name for the spawned agent (for tracking/identification)
- `team_name` (optional): Team context for spawning. Uses current team context if omitted.
- `mode` (optional): Permission mode for the spawned agent

**Permission Mode Values (for `mode`):**
| Mode | Description |
|------|-------------|
| `"default"` | Standard behavior, prompts for dangerous operations |
| `"acceptEdits"` | Auto-accept file edit operations |
| `"bypassPermissions"` | Bypass all permission checks |
| `"plan"` | Planning mode, requires plan approval before execution |
| `"delegate"` | Delegate permission decisions to parent |
| `"dontAsk"` | Don't prompt for permissions, deny if not pre-approved |

**Example:**
```json
{
  "description": "Review auth module",
  "prompt": "Review the authentication module for security issues...",
  "subagent_type": "code-reviewer",
  "model": "sonnet",
  "run_in_background": true,
  "name": "auth-reviewer",
  "mode": "plan"
}
```

### TaskOutput

Retrieves output from a running or completed background task (agent or shell).

**Parameters:**
- `task_id` (required): The task ID to get output from
- `block` (required): Whether to wait for task completion (`true`) or return immediately (`false`)
- `timeout` (required): Maximum wait time in milliseconds

**Example:**
```json
{
  "task_id": "abc123",
  "block": true,
  "timeout": 30000
}
```

### TaskStop

Stops a running background task.

**Parameters:**
- `task_id` (optional): The ID of the background task to stop
- `shell_id` (optional, deprecated): Use `task_id` instead

## Task Notification Messages

When a background task completes, the CLI emits a `SDKTaskNotificationMessage`:

```typescript
type SDKTaskNotificationMessage = {
    type: 'system';
    subtype: 'task_notification';
    task_id: string;
    status: 'completed' | 'failed' | 'stopped';
    output_file: string;
    summary: string;
    uuid: string;
    session_id: string;
};
```

The Go SDK surfaces this as `TaskNotificationMessage` in the message stream.

## TodoWrite vs TaskCreate

Claude Code has two task tracking mechanisms:

| Feature | `TodoWrite` (Legacy) | `TaskCreate`/`TaskUpdate`/`TaskList` |
|---------|---------------------|--------------------------------------|
| Task IDs | No (index-based) | Yes (auto-generated) |
| Dependencies | No | Yes (`blocks`/`blockedBy`) |
| Metadata | No | Yes (arbitrary key-value) |
| Owner | No | Yes |
| Persistence | In-memory | File-based (`~/.claude/tasks/`) |
| Multi-agent | No | Yes (shared task lists) |
| Schema | `{ content, status, activeForm }` | Full task object |

The `TodoWrite` tool takes a flat array of `{ content, status, activeForm }` objects and replaces the entire list. It is suitable for simple checklists but lacks the structure needed for multi-agent coordination. New usage should prefer `TaskCreate`/`TaskUpdate`/`TaskList`.

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

### Relevant Hook Events

| Hook Event | When Fired | Relevance |
|------------|------------|-----------|
| `SubagentStart` | Agent spawned via Task tool | Intercept agent creation |
| `SubagentStop` | Agent completes or is stopped | React to agent completion |
| `PermissionRequest` | Tool needs permission approval | Control tool access for agents |
| `Setup` | Session initialization | Configure task environment |

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

### Spawning Named Agents

As of v0.2.30, the Task (Agent) tool supports `name` and `team_name` fields for agent identification:

```json
{
  "description": "Run tests",
  "prompt": "Run the full test suite and report results",
  "subagent_type": "test-runner",
  "name": "test-agent-1",
  "team_name": "qa-team",
  "run_in_background": true
}
```

This enables better tracking when multiple agents are running concurrently.

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

### Background Agent Workflow

```
User: "Analyze these three modules in parallel"

Claude:
1. Task (Agent): spawn "analyzer" for module A (run_in_background: true) -> task_id: "t1"
2. Task (Agent): spawn "analyzer" for module B (run_in_background: true) -> task_id: "t2"
3. Task (Agent): spawn "analyzer" for module C (run_in_background: true) -> task_id: "t3"

4. TaskOutput: { task_id: "t1", block: true, timeout: 60000 } -> results
5. TaskOutput: { task_id: "t2", block: true, timeout: 60000 } -> results
6. TaskOutput: { task_id: "t3", block: true, timeout: 60000 } -> results

7. Synthesize results from all three agents
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
