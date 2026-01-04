# Hooks

Hooks let you intercept and respond to events during Claude's execution. You
can log activity, validate inputs, modify behavior, or block operations
entirely.

## Hook Types

The SDK supports twelve hook types that fire at different points:

| Hook | When It Fires |
|------|---------------|
| `PreToolUse` | Before a tool executes |
| `PostToolUse` | After a tool completes successfully |
| `PostToolUseFailure` | After a tool fails |
| `UserPromptSubmit` | When the user sends a message |
| `SessionStart` | When a session begins |
| `SessionEnd` | When a session ends |
| `Stop` | When Claude stops generating |
| `SubagentStart` | When a subagent is spawned |
| `SubagentStop` | When a subagent finishes |
| `PreCompact` | Before context compaction |
| `Notification` | When Claude sends a notification |
| `PermissionRequest` | When a permission check is requested |

## Basic Usage

Register hooks when creating a client:

```go
client, _ := goclaude.NewClient(
    goclaude.WithHooks(map[goclaude.HookType][]goclaude.HookConfig{
        goclaude.HookTypePreToolUse: {
            {
                Matcher:  "*",
                Callback: logToolCall,
            },
        },
    }),
)
```

The `Matcher` field is a glob pattern. Use `"*"` to match all tools, `"Bash"`
for exact matches, or `"Read*"` for prefix matching.

## Writing Callbacks

Callbacks receive typed inputs and return `HookResult`:

```go
func logToolCall(
    ctx context.Context,
    input goclaude.HookInput,
) (goclaude.HookResult, error) {
    // Type assert to the specific input type
    pre := input.(goclaude.PreToolUseInput)

    log.Printf("Tool: %s", pre.ToolName)
    log.Printf("Input: %s", string(pre.ToolInput))

    // Continue execution
    return goclaude.HookResult{Continue: true}, nil
}
```

Set `Continue: false` to block the operation:

```go
func blockDangerousCommands(
    ctx context.Context,
    input goclaude.HookInput,
) (goclaude.HookResult, error) {
    pre := input.(goclaude.PreToolUseInput)

    if pre.ToolName == "Bash" {
        var bashInput struct {
            Command string `json:"command"`
        }
        json.Unmarshal(pre.ToolInput, &bashInput)

        if strings.Contains(bashInput.Command, "rm -rf") {
            return goclaude.HookResult{Continue: false}, nil
        }
    }

    return goclaude.HookResult{Continue: true}, nil
}
```

## Hook Input Types

Each hook type has a corresponding input struct. All embed `BaseHookInput`:

```go
type BaseHookInput struct {
    SessionID      string // Current session ID
    TranscriptPath string // Path to transcript file
    Cwd            string // Current working directory
    PermissionMode string // Active permission mode
}
```

Access base fields via the `Base()` method:

```go
func myHook(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
    base := input.Base()
    log.Printf("Session: %s, CWD: %s", base.SessionID, base.Cwd)
    // ...
}
```

### PreToolUseInput

Fires before tool execution.

```go
type PreToolUseInput struct {
    BaseHookInput
    ToolName  string          // Name of the tool being called
    ToolInput json.RawMessage // Tool arguments as JSON
}
```

Example: Log all file reads:

```go
goclaude.HookTypePreToolUse: {
    {
        Matcher: "Read",
        Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
            pre := input.(goclaude.PreToolUseInput)
            var readInput struct {
                FilePath string `json:"file_path"`
            }
            json.Unmarshal(pre.ToolInput, &readInput)
            log.Printf("Reading file: %s", readInput.FilePath)
            return goclaude.HookResult{Continue: true}, nil
        },
    },
},
```

### PostToolUseInput

Fires after successful tool execution.

```go
type PostToolUseInput struct {
    BaseHookInput
    ToolName     string          // Name of the tool
    ToolInput    json.RawMessage // Tool arguments
    ToolResponse json.RawMessage // Tool result
}
```

Example: Track file modifications:

```go
goclaude.HookTypePostToolUse: {
    {
        Matcher: "Write",
        Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
            post := input.(goclaude.PostToolUseInput)
            var writeInput struct {
                FilePath string `json:"file_path"`
            }
            json.Unmarshal(post.ToolInput, &writeInput)
            modifiedFiles = append(modifiedFiles, writeInput.FilePath)
            return goclaude.HookResult{Continue: true}, nil
        },
    },
},
```

### PostToolUseFailureInput

Fires when tool execution fails.

```go
type PostToolUseFailureInput struct {
    BaseHookInput
    ToolName    string          // Name of the tool
    ToolInput   json.RawMessage // Tool arguments
    Error       string          // Error message
    IsInterrupt bool            // Whether it was interrupted
}
```

Example: Alert on failures:

```go
goclaude.HookTypePostToolUseFailure: {
    {
        Matcher: "*",
        Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
            failure := input.(goclaude.PostToolUseFailureInput)
            alerter.Send(fmt.Sprintf("Tool %s failed: %s", failure.ToolName, failure.Error))
            return goclaude.HookResult{Continue: true}, nil
        },
    },
},
```

### UserPromptSubmitInput

Fires when the user sends a message.

```go
type UserPromptSubmitInput struct {
    BaseHookInput
    Prompt string // The user's message
}
```

Example: Content filtering:

```go
goclaude.HookTypeUserPromptSubmit: {
    {
        Matcher: "*",
        Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
            prompt := input.(goclaude.UserPromptSubmitInput)
            if containsPII(prompt.Prompt) {
                log.Printf("Blocked prompt containing PII")
                return goclaude.HookResult{Continue: false}, nil
            }
            return goclaude.HookResult{Continue: true}, nil
        },
    },
},
```

### SessionStartInput / SessionEndInput

Fire at session lifecycle boundaries.

```go
type SessionStartInput struct {
    BaseHookInput
    Source string // "startup", "resume", "clear", or "compact"
}

type SessionEndInput struct {
    BaseHookInput
    Reason string // Exit reason
}
```

Example: Track session duration:

```go
var sessionStart time.Time

goclaude.HookTypeSessionStart: {
    {
        Matcher: "*",
        Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
            start := input.(goclaude.SessionStartInput)
            sessionStart = time.Now()
            log.Printf("Session started (source: %s)", start.Source)
            return goclaude.HookResult{Continue: true}, nil
        },
    },
},
goclaude.HookTypeSessionEnd: {
    {
        Matcher: "*",
        Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
            end := input.(goclaude.SessionEndInput)
            duration := time.Since(sessionStart)
            log.Printf("Session ended after %v (reason: %s)", duration, end.Reason)
            return goclaude.HookResult{Continue: true}, nil
        },
    },
},
```

### PreCompactInput

Fires before context compaction.

```go
type PreCompactInput struct {
    BaseHookInput
    Trigger      string // "manual" or "auto"
    MessageCount int    // Messages being compacted
}
```

Example: Save state before compaction:

```go
goclaude.HookTypePreCompact: {
    {
        Matcher: "*",
        Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
            compact := input.(goclaude.PreCompactInput)
            log.Printf("Compacting %d messages (trigger: %s)", compact.MessageCount, compact.Trigger)
            saveCheckpoint()
            return goclaude.HookResult{Continue: true}, nil
        },
    },
},
```

### SubagentStartInput / SubagentStopInput

Fire during subagent lifecycle.

```go
type SubagentStartInput struct {
    BaseHookInput
    AgentID   string
    AgentType string
}

type SubagentStopInput struct {
    BaseHookInput
    AgentName string
    Status    string // "success" or "error"
    Result    string
}
```

### NotificationInput

Fires when Claude sends notifications.

```go
type NotificationInput struct {
    BaseHookInput
    Message string
    Title   string
}
```

### PermissionRequestInput

Fires when a permission check is requested.

```go
type PermissionRequestInput struct {
    BaseHookInput
    ToolName              string
    ToolInput             json.RawMessage
    PermissionSuggestions []PermissionUpdate
}
```

## Modifying Behavior

The `Modify` field in `HookResult` lets you alter inputs or outputs:

```go
func sanitizeCommand(
    ctx context.Context,
    input goclaude.HookInput,
) (goclaude.HookResult, error) {
    pre := input.(goclaude.PreToolUseInput)

    if pre.ToolName == "Bash" {
        var bashInput map[string]interface{}
        json.Unmarshal(pre.ToolInput, &bashInput)

        // Modify the command
        if cmd, ok := bashInput["command"].(string); ok {
            bashInput["command"] = sanitize(cmd)
        }

        return goclaude.HookResult{
            Continue: true,
            Modify:   bashInput,
        }, nil
    }

    return goclaude.HookResult{Continue: true}, nil
}
```

## Multiple Hooks

Register multiple callbacks for the same hook type:

```go
goclaude.HookTypePreToolUse: {
    {Matcher: "*", Callback: logAllTools},
    {Matcher: "Bash", Callback: validateBashCommands},
    {Matcher: "Write", Callback: validateFileWrites},
},
```

Callbacks execute in order. If any returns `Continue: false`, subsequent
callbacks don't run.

## Complete Example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "time"

    "github.com/roasbeef/goclaude"
)

func main() {
    client, err := goclaude.NewClient(
        goclaude.WithSystemPrompt("You are a helpful assistant."),
        goclaude.WithHooks(map[goclaude.HookType][]goclaude.HookConfig{
            goclaude.HookTypePreToolUse: {
                {Matcher: "*", Callback: logToolStart},
                {Matcher: "Bash", Callback: validateBash},
            },
            goclaude.HookTypePostToolUse: {
                {Matcher: "*", Callback: logToolComplete},
            },
            goclaude.HookTypeSessionStart: {
                {Matcher: "*", Callback: onSessionStart},
            },
        }),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()
    for msg := range client.Query(ctx, "List files in the current directory") {
        if m, ok := msg.(goclaude.AssistantMessage); ok {
            fmt.Println(m.ContentText())
        }
    }
}

func logToolStart(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
    pre := input.(goclaude.PreToolUseInput)
    log.Printf("[%s] Starting tool: %s", time.Now().Format("15:04:05"), pre.ToolName)
    return goclaude.HookResult{Continue: true}, nil
}

func logToolComplete(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
    post := input.(goclaude.PostToolUseInput)
    log.Printf("[%s] Completed tool: %s", time.Now().Format("15:04:05"), post.ToolName)
    return goclaude.HookResult{Continue: true}, nil
}

func validateBash(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
    pre := input.(goclaude.PreToolUseInput)

    var bashInput struct {
        Command string `json:"command"`
    }
    json.Unmarshal(pre.ToolInput, &bashInput)

    // Block dangerous patterns
    dangerous := []string{"rm -rf /", "mkfs", "dd if=", "> /dev/"}
    for _, pattern := range dangerous {
        if strings.Contains(bashInput.Command, pattern) {
            log.Printf("Blocked dangerous command: %s", bashInput.Command)
            return goclaude.HookResult{Continue: false}, nil
        }
    }

    return goclaude.HookResult{Continue: true}, nil
}

func onSessionStart(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
    start := input.(goclaude.SessionStartInput)
    log.Printf("Session %s started (source: %s)", start.SessionID, start.Source)
    return goclaude.HookResult{Continue: true}, nil
}
```

## See Also

- [Permissions](permissions.md) - Alternative approach to controlling Claude's actions
- [MCP Tools](mcp-tools.md) - Create tools that hooks can intercept
