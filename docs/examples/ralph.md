# Ralph Wiggum Loop

The Ralph Wiggum loop is an iterative AI development pattern where Claude
repeatedly works on a task until a completion signal is detected. Named after
the Simpsons character who persists despite obstacles, this pattern enables
autonomous multi-iteration workflows.

## How It Works

1. User provides a task and a completion promise (e.g., "TASK COMPLETE")
2. Claude works on the task, attempting to exit when done
3. A Stop hook intercepts the exit attempt
4. If the completion promise isn't found, the hook reinjects the original prompt
5. Repeat until the promise appears or max iterations is reached

Claude sees its previous work (modified files, git commits, etc.) each
iteration, allowing it to make incremental progress on complex tasks.

## Basic Usage

```go
loop := claudeagent.NewRalphLoop(claudeagent.RalphConfig{
    Task:              "Build a REST API with full test coverage",
    CompletionPromise: "TASK COMPLETE",
    MaxIterations:     10,
})

for iter := range loop.Run(ctx, claudeagent.WithModel("claude-sonnet-4-5-20250929")) {
    fmt.Printf("Iteration %d\n", iter.Number)

    for _, msg := range iter.Messages {
        if m, ok := msg.(claudeagent.AssistantMessage); ok {
            fmt.Println(m.ContentText())
        }
    }

    if iter.Complete {
        fmt.Println("Task completed!")
        break
    }
}
```

## Configuration

```go
type RalphConfig struct {
    // Task is the initial prompt for Claude.
    Task string

    // CompletionPromise is the signal text indicating task completion.
    // Claude should output this wrapped in <promise></promise> tags.
    // Default: "TASK COMPLETE"
    CompletionPromise string

    // MaxIterations prevents infinite loops.
    // Default: 10
    MaxIterations int
}
```

## Iteration Results

Each iteration yields an `Iteration` struct:

```go
type Iteration struct {
    Number       int        // 1-based iteration number
    Messages     []Message  // All messages from this iteration
    Complete     bool       // True if completion promise was detected
    Error        error      // Set if an error occurred
    SessionID    string     // CLI session ID
    CostUSD      float64    // Cost of this iteration
    TotalCostUSD float64    // Cumulative cost
}
```

## Completion Detection

Claude signals completion by outputting the promise in XML tags:

```
I've finished implementing the REST API with tests.
<promise>TASK COMPLETE</promise>
```

The SDK detects this pattern and sets `iter.Complete = true`.

## Example: Code Review Loop

```go
loop := claudeagent.NewRalphLoop(claudeagent.RalphConfig{
    Task: `Review and fix all issues in the codebase:
           1. Run tests and fix any failures
           2. Run linter and fix warnings
           3. Check for security issues

           When all issues are resolved, output your completion signal.`,
    CompletionPromise: "ALL_ISSUES_FIXED",
    MaxIterations:     15,
})

opts := []claudeagent.Option{
    claudeagent.WithSystemPrompt(
        "You are a senior developer performing code review. " +
        "Be thorough and fix all issues before completing.",
    ),
    claudeagent.WithPermissionMode(claudeagent.PermissionModeAcceptEdits),
}

var totalCost float64
for iter := range loop.Run(ctx, opts...) {
    log.Printf("Iteration %d: %d messages", iter.Number, len(iter.Messages))
    totalCost = iter.TotalCostUSD

    if iter.Error != nil {
        log.Fatal(iter.Error)
    }
    if iter.Complete {
        log.Println("Code review complete!")
        break
    }
}

log.Printf("Total cost: $%.4f", totalCost)
```

## Example: With MCP Tools

Combine Ralph loops with in-process MCP tools:

```go
type DeployArgs struct {
    Environment string `json:"environment"`
}

server := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{
    Name: "deployment",
    Tools: []claudeagent.ToolRegistrar{
        claudeagent.Tool("deploy", "Deploy to environment",
            func(ctx context.Context, args DeployArgs) (claudeagent.ToolResult, error) {
                // Deployment logic...
                return claudeagent.TextResult("Deployed successfully"), nil
            },
        ),
    },
})

loop := claudeagent.NewRalphLoop(claudeagent.RalphConfig{
    Task:              "Deploy to staging, run smoke tests, then deploy to production",
    CompletionPromise: "DEPLOY_COMPLETE",
    MaxIterations:     10,
})

opts := []claudeagent.Option{
    claudeagent.WithMcpServer("deployment", server),
    claudeagent.WithPermissionMode(claudeagent.PermissionModeBypassAll),
    claudeagent.WithAllowDangerouslySkipPermissions(true),
}

for iter := range loop.Run(ctx, opts...) {
    if iter.Complete {
        log.Println("Deployment pipeline complete!")
        break
    }
}
```

## State Inspection

Query loop state at any time:

```go
loop := claudeagent.NewRalphLoop(cfg)

// Start loop in goroutine
go func() {
    for iter := range loop.Run(ctx, opts...) {
        // Process iterations...
    }
}()

// Check state from main goroutine
time.Sleep(30 * time.Second)
fmt.Printf("Current iteration: %d\n", loop.CurrentIteration())
fmt.Printf("Complete: %v\n", loop.IsComplete())
fmt.Printf("Total cost: $%.4f\n", loop.TotalCost())
```

## Stop Hook Internals

The Ralph loop works by registering a Stop hook that intercepts session exit:

```go
stopHook := func(ctx context.Context, input HookInput) (HookResult, error) {
    if complete || iteration >= maxIterations {
        // Allow exit
        return HookResult{
            Continue: true,
            Decision: "approve",
        }, nil
    }

    // Block exit and reinject prompt
    return HookResult{
        Continue:      false,
        Decision:      "block",
        Reason:        buildNextPrompt(),
        SystemMessage: fmt.Sprintf("Iteration %d of %d", iteration, maxIterations),
    }, nil
}
```

The `Decision`, `Reason`, and `SystemMessage` fields are specific to Stop hooks:

- `Decision: "block"` prevents the session from exiting
- `Reason` is reinjected as the next user prompt
- `SystemMessage` provides context to Claude about the loop state

## Best Practices

**Clear task description**: Be explicit about what "done" means:

```go
// Good
Task: "Implement user authentication with login, logout, and password reset. " +
      "Write tests for each endpoint. " +
      "When all tests pass, output your completion signal."

// Vague
Task: "Add authentication to the app"
```

**Appropriate iteration limits**: Start conservative and adjust:

```go
MaxIterations: 5   // Simple tasks
MaxIterations: 10  // Medium complexity
MaxIterations: 20  // Complex multi-step workflows
```

**Monitor costs**: Check `TotalCostUSD` each iteration:

```go
for iter := range loop.Run(ctx, opts...) {
    if iter.TotalCostUSD > 5.0 {
        log.Println("Cost limit reached, stopping")
        break
    }
}
```

**Handle errors gracefully**: Check `iter.Error`:

```go
for iter := range loop.Run(ctx, opts...) {
    if iter.Error != nil {
        log.Printf("Iteration error: %v", iter.Error)
        break
    }
}
```

## Comparison to Subagents

| Feature | Ralph Loop | Subagents |
|---------|------------|-----------|
| Persistence | Same session, sees prior work | Fresh context each invocation |
| Control | High (iteration-level) | Low (fire and forget) |
| Use case | Complex iterative tasks | Specialized sub-tasks |
| State | Shared across iterations | Isolated per agent |

Ralph loops excel when Claude needs to see and build on its previous work.
Subagents are better for isolated, specialized tasks that don't need shared
context.

## Related

- [Hooks](hooks.md) - The Stop hook that powers Ralph loops
- [Sessions](sessions.md) - Session persistence across iterations
- [MCP Tools](mcp-tools.md) - Adding custom tools to Ralph workflows
