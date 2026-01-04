# Sessions

Sessions let you persist and resume conversations across process restarts.
Claude maintains context, so you can continue where you left off days or weeks
later.

## How Sessions Work

Every conversation has a session ID. The Claude Code CLI stores session data
(messages, context, file states) locally. When you resume a session, Claude has
access to the entire conversation history.

```
New Session:
    client.Query("Review this PR") → session_abc123 created

Later:
    client.WithResume("session_abc123") → context restored
    client.Query("What issues did you find?") → continues conversation
```

## Getting the Session ID

Session IDs are available from streams:

```go
client, _ := goclaude.NewClient()
defer client.Close()

stream, _ := client.Stream(ctx)

// Send first message
stream.Send(ctx, "Let's start a code review.")

// Get session ID for later
sessionID := stream.SessionID()
fmt.Printf("Session ID: %s\n", sessionID)

// Process responses...
for msg := range stream.Messages() {
    // handle messages
}
```

Store the session ID however makes sense for your application: database, file,
environment variable, etc.

## Resuming Sessions

Pass the session ID when creating a new client:

```go
// Later, in a new process
client, _ := goclaude.NewClient(
    goclaude.WithResume(sessionID),
)
defer client.Close()

stream, _ := client.Stream(ctx)

// Claude remembers the conversation
stream.Send(ctx, "What were your main findings?")
```

Claude will respond with context from the original conversation.

## Forking Sessions

Forking creates a new session that branches from an existing one. The new
session shares history up to the fork point but diverges afterward.

```go
// Fork from an existing session
client, _ := goclaude.NewClient(
    goclaude.WithForkSession(originalSessionID),
)
```

This is useful for:
- Exploring alternative approaches without losing the original conversation
- Creating checkpoints you can return to
- Running "what if" scenarios

## Resume at Specific Message

For fine-grained control, resume at a specific message UUID:

```go
client, _ := goclaude.NewClient(
    goclaude.WithResume(sessionID),
    goclaude.WithResumeSessionAt(messageUUID),
)
```

Messages after that UUID are discarded, effectively rewinding the conversation.

## Fork on Resume

Create a new session ID when resuming (fork behavior):

```go
client, _ := goclaude.NewClient(
    goclaude.WithResume(sessionID),
    goclaude.WithForkOnResume(true),
)
```

This preserves the original session unchanged while continuing in a new one.

## File Checkpointing

Enable file checkpointing to track file modifications:

```go
client, _ := goclaude.NewClient(
    goclaude.WithEnableFileCheckpointing(true),
)

stream, _ := client.Stream(ctx)
stream.Send(ctx, "Refactor the authentication module.")

// Claude modifies files...

// Later, revert to state before a specific message
stream.RewindFiles(ctx, userMessageUUID)
```

This restores all files Claude modified to their state at that point.

## Continue Previous Session

The `WithContinue` option continues the most recent session without knowing its ID:

```go
client, _ := goclaude.NewClient(
    goclaude.WithContinue(true),
)
```

Useful for CLI tools where users expect to pick up where they left off.

## Session Lifecycle Hooks

Track session lifecycle with hooks:

```go
client, _ := goclaude.NewClient(
    goclaude.WithHooks(map[goclaude.HookType][]goclaude.HookConfig{
        goclaude.HookTypeSessionStart: {
            {
                Matcher: "*",
                Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
                    start := input.(goclaude.SessionStartInput)
                    log.Printf("Session started: %s (source: %s)", start.SessionID, start.Source)
                    // Source is "startup", "resume", "clear", or "compact"
                    return goclaude.HookResult{Continue: true}, nil
                },
            },
        },
        goclaude.HookTypeSessionEnd: {
            {
                Matcher: "*",
                Callback: func(ctx context.Context, input goclaude.HookInput) (goclaude.HookResult, error) {
                    end := input.(goclaude.SessionEndInput)
                    log.Printf("Session ended: reason=%s", end.Reason)
                    return goclaude.HookResult{Continue: true}, nil
                },
            },
        },
    }),
)
```

## Practical Example: Code Review Bot

Here's a complete example of a code review bot that persists sessions:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"

    "github.com/roasbeef/goclaude"
)

const sessionFile = "review_session.json"

type ReviewState struct {
    SessionID string `json:"session_id"`
    PRNumber  int    `json:"pr_number"`
}

func main() {
    ctx := context.Background()

    // Check for existing session
    state := loadState()

    var client *goclaude.Client
    var err error

    if state != nil && os.Args[1] == "continue" {
        // Resume existing review
        client, err = goclaude.NewClient(
            goclaude.WithResume(state.SessionID),
            goclaude.WithSystemPrompt("You are a code reviewer."),
        )
        log.Printf("Resuming review of PR #%d", state.PRNumber)
    } else {
        // Start new review
        client, err = goclaude.NewClient(
            goclaude.WithSystemPrompt("You are a code reviewer."),
        )
    }
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    stream, _ := client.Stream(ctx)

    // Save session ID
    if state == nil {
        prNumber := 123 // from args
        state = &ReviewState{
            SessionID: stream.SessionID(),
            PRNumber:  prNumber,
        }
        saveState(state)
        stream.Send(ctx, fmt.Sprintf("Review PR #%d: https://github.com/org/repo/pull/%d", prNumber, prNumber))
    } else {
        stream.Send(ctx, os.Args[2]) // follow-up question
    }

    for msg := range stream.Messages() {
        switch m := msg.(type) {
        case goclaude.AssistantMessage:
            fmt.Println(m.ContentText())
        case goclaude.ResultMessage:
            fmt.Printf("\n[Review complete. Cost: $%.4f]\n", m.TotalCostUSD)
        }
    }
}

func loadState() *ReviewState {
    data, err := os.ReadFile(sessionFile)
    if err != nil {
        return nil
    }
    var state ReviewState
    json.Unmarshal(data, &state)
    return &state
}

func saveState(state *ReviewState) {
    data, _ := json.Marshal(state)
    os.WriteFile(sessionFile, data, 0644)
}
```

Usage:
```bash
# Start new review
./reviewer new 123

# Continue review with follow-up
./reviewer continue "What about the error handling?"

# Continue again
./reviewer continue "Can you check the test coverage?"
```

## Best Practices

**Store session IDs reliably.** Use a database or persistent storage, not just memory.

**Handle missing sessions gracefully.** The session might have been deleted or expired.

```go
client, err := goclaude.NewClient(
    goclaude.WithResume(sessionID),
)
if err != nil {
    var notFound *goclaude.ErrSessionNotFound
    if errors.As(err, &notFound) {
        // Session doesn't exist, start fresh
        client, _ = goclaude.NewClient()
    } else {
        return err
    }
}
```

**Use file checkpointing for destructive operations.** If Claude might modify
important files, enable checkpointing so you can revert.

**Consider fork vs resume semantics.** Resume modifies the existing session;
fork creates a new one. Choose based on whether you want to preserve history.

## See Also

- [Hooks](hooks.md) - Respond to session lifecycle events
- [Streaming](streaming.md) - Working with streams
