# Permissions

The permission system controls what Claude can do during a conversation. You
can set broad modes or make fine-grained decisions on each tool invocation.

## Permission Modes

Four modes control Claude's default behavior:

### Default Mode

Standard permission checks. Claude asks before performing sensitive operations.

```go
client, _ := goclaude.NewClient(
    goclaude.WithPermissionMode(goclaude.PermissionModeDefault),
)
```

### Plan Mode

Claude can only plan and describe what it would do. No tools execute.

```go
client, _ := goclaude.NewClient(
    goclaude.WithPermissionMode(goclaude.PermissionModePlan),
)
```

Useful for previewing changes before committing to them.

### Accept Edits Mode

Auto-approve file read/write operations. Claude can modify files without asking.

```go
client, _ := goclaude.NewClient(
    goclaude.WithPermissionMode(goclaude.PermissionModeAcceptEdits),
)
```

Use when you trust Claude's file operations or are working in a sandbox.

### Bypass All Mode

Skip all permission checks. Requires explicit opt-in.

```go
client, _ := goclaude.NewClient(
    goclaude.WithPermissionMode(goclaude.PermissionModeBypassAll),
    goclaude.WithAllowDangerouslySkipPermissions(true), // required
)
```

Only use in fully trusted environments (CI pipelines, sandboxed containers, etc.).

## Runtime Permission Callback

For fine-grained control, provide a callback that decides on each tool
invocation:

```go
client, _ := goclaude.NewClient(
    goclaude.WithCanUseTool(func(ctx context.Context, req goclaude.ToolPermissionRequest) goclaude.PermissionResult {
        // Inspect req.ToolName and req.Arguments
        // Return PermissionAllow{} or PermissionDeny{Reason: "..."}
    }),
)
```

The callback receives:

```go
type ToolPermissionRequest struct {
    ToolName  string          // e.g., "Bash", "Write", "Read"
    Arguments json.RawMessage // tool arguments as JSON
    Context   PermissionContext
}

type PermissionContext struct {
    SessionID string
    Metadata  map[string]interface{}
}
```

### Example: Block System Paths

```go
goclaude.WithCanUseTool(func(ctx context.Context, req goclaude.ToolPermissionRequest) goclaude.PermissionResult {
    if req.ToolName == "Write" || req.ToolName == "Edit" {
        var input struct {
            FilePath string `json:"file_path"`
        }
        json.Unmarshal(req.Arguments, &input)

        blockedPaths := []string{"/etc", "/usr", "/bin", "/sbin", "/var"}
        for _, blocked := range blockedPaths {
            if strings.HasPrefix(input.FilePath, blocked) {
                return goclaude.PermissionDeny{
                    Reason: fmt.Sprintf("writes to %s are not allowed", blocked),
                }
            }
        }
    }
    return goclaude.PermissionAllow{}
}),
```

### Example: Whitelist Commands

```go
goclaude.WithCanUseTool(func(ctx context.Context, req goclaude.ToolPermissionRequest) goclaude.PermissionResult {
    if req.ToolName == "Bash" {
        var input struct {
            Command string `json:"command"`
        }
        json.Unmarshal(req.Arguments, &input)

        allowed := []string{"go ", "git ", "make ", "npm ", "cargo "}
        for _, prefix := range allowed {
            if strings.HasPrefix(input.Command, prefix) {
                return goclaude.PermissionAllow{}
            }
        }
        return goclaude.PermissionDeny{
            Reason: "only go, git, make, npm, and cargo commands are allowed",
        }
    }
    return goclaude.PermissionAllow{}
}),
```

### Example: Require Confirmation

```go
goclaude.WithCanUseTool(func(ctx context.Context, req goclaude.ToolPermissionRequest) goclaude.PermissionResult {
    if req.ToolName == "Write" {
        var input struct {
            FilePath string `json:"file_path"`
        }
        json.Unmarshal(req.Arguments, &input)

        fmt.Printf("Claude wants to write to %s. Allow? [y/N] ", input.FilePath)
        var response string
        fmt.Scanln(&response)

        if strings.ToLower(response) != "y" {
            return goclaude.PermissionDeny{Reason: "user declined"}
        }
    }
    return goclaude.PermissionAllow{}
}),
```

## Tool Allow/Disallow Lists

Restrict which tools are available:

```go
// Only allow these tools
client, _ := goclaude.NewClient(
    goclaude.WithAllowedTools([]string{"Read", "Glob", "Grep"}),
)

// Block specific tools
client, _ := goclaude.NewClient(
    goclaude.WithDisallowedTools([]string{"Bash", "Write", "Edit"}),
)
```

## Dynamic Permission Mode

Change permission mode during a session:

```go
stream, _ := client.Stream(ctx)

// Start in plan mode
stream.SetPermissionMode(ctx, goclaude.PermissionModePlan)
stream.Send(ctx, "How would you refactor this code?")

// Review the plan...

// Switch to accept edits for execution
stream.SetPermissionMode(ctx, goclaude.PermissionModeAcceptEdits)
stream.Send(ctx, "Go ahead and implement that.")
```

## Sandbox Settings

Configure sandbox behavior for command execution:

```go
client, _ := goclaude.NewClient(
    goclaude.WithSandbox(&goclaude.SandboxSettings{
        Enabled:                  true,
        AutoAllowBashIfSandboxed: true,
        ExcludedCommands:         []string{"docker", "kubectl"},
        Network: &goclaude.NetworkSandboxSettings{
            AllowLocalBinding:   true,
            AllowAllUnixSockets: false,
            AllowUnixSockets:    []string{"/var/run/docker.sock"},
        },
    }),
)
```

Settings:

| Field | Description |
|-------|-------------|
| `Enabled` | Enable sandbox mode |
| `AutoAllowBashIfSandboxed` | Auto-approve Bash when sandboxed |
| `ExcludedCommands` | Commands that bypass sandbox |
| `AllowUnsandboxedCommands` | Allow model to request unsandboxed execution |
| `Network.AllowLocalBinding` | Allow binding to local ports |
| `Network.AllowUnixSockets` | Specific Unix sockets to allow |
| `Network.AllowAllUnixSockets` | Allow all Unix socket access |

## Permission Hooks

Use hooks for more complex permission logic:

```go
goclaude.WithHooks(map[goclaude.HookType][]goclaude.HookConfig{
    goclaude.HookTypePermissionRequest: {
        {
            Matcher: "*",
            Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
                perm := input.(goclaude.PermissionRequestInput)

                // Log all permission requests
                log.Printf("Permission requested for %s", perm.ToolName)

                // Check suggestions from Claude
                for _, suggestion := range perm.PermissionSuggestions {
                    log.Printf("Suggestion: %s", suggestion.Type)
                }

                return goclaude.HookResult{Continue: true}, nil
            },
        },
    },
}),
```

## Complete Example

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"

    "github.com/roasbeef/goclaude"
)

func main() {
    // Get allowed directory from environment
    allowedDir := os.Getenv("CLAUDE_ALLOWED_DIR")
    if allowedDir == "" {
        allowedDir = "."
    }
    allowedDir, _ = filepath.Abs(allowedDir)

    client, err := goclaude.NewClient(
        goclaude.WithSystemPrompt("You are a code assistant."),
        goclaude.WithPermissionMode(goclaude.PermissionModeDefault),
        goclaude.WithCanUseTool(makePermissionChecker(allowedDir)),
        goclaude.WithDisallowedTools([]string{"WebFetch", "WebSearch"}),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()
    for msg := range client.Query(ctx, "Help me refactor main.go") {
        if m, ok := msg.(goclaude.AssistantMessage); ok {
            fmt.Println(m.ContentText())
        }
    }
}

func makePermissionChecker(allowedDir string) goclaude.CanUseToolFunc {
    return func(ctx context.Context, req goclaude.ToolPermissionRequest) goclaude.PermissionResult {
        switch req.ToolName {
        case "Read", "Write", "Edit", "Glob", "Grep":
            // File operations must be within allowed directory
            var input struct {
                FilePath string `json:"file_path"`
                Path     string `json:"path"`
                Pattern  string `json:"pattern"`
            }
            json.Unmarshal(req.Arguments, &input)

            path := input.FilePath
            if path == "" {
                path = input.Path
            }
            if path == "" {
                return goclaude.PermissionAllow{}
            }

            absPath, _ := filepath.Abs(path)
            if !strings.HasPrefix(absPath, allowedDir) {
                return goclaude.PermissionDeny{
                    Reason: fmt.Sprintf("access restricted to %s", allowedDir),
                }
            }

        case "Bash":
            // Only allow specific commands
            var input struct {
                Command string `json:"command"`
            }
            json.Unmarshal(req.Arguments, &input)

            safeCommands := []string{
                "go build", "go test", "go fmt", "go vet",
                "git status", "git diff", "git log",
                "ls", "cat", "head", "tail", "wc",
            }

            for _, safe := range safeCommands {
                if strings.HasPrefix(input.Command, safe) {
                    return goclaude.PermissionAllow{}
                }
            }

            return goclaude.PermissionDeny{
                Reason: "only go, git, and basic file commands are allowed",
            }
        }

        return goclaude.PermissionAllow{}
    }
}
```

## Best Practices

**Start restrictive, relax as needed.** Begin with plan mode or a whitelist,
then grant more permissions as you gain confidence.

**Log permission decisions.** Track what Claude tries to do, even if allowed.
This helps debug unexpected behavior.

**Use meaningful denial reasons.** Claude uses these to explain to users why
something failed and to adjust its approach.

**Combine approaches.** Use modes for broad control, callbacks for fine-grained
decisions, and hooks for logging.

## See Also

- [Hooks](hooks.md) - Intercept operations before they execute
- [MCP Tools](mcp-tools.md) - Control tool availability at definition time
