# Streaming

This guide covers real-time response handling, multi-turn conversations, and
advanced streaming patterns.

## Basic Streaming

The `Query` method returns an iterator that yields messages as they arrive:

```go
for msg := range client.Query(ctx, "Tell me a story.") {
    switch m := msg.(type) {
    case goclaude.AssistantMessage:
        fmt.Println(m.ContentText())
    case goclaude.ResultMessage:
        fmt.Printf("Done. Cost: $%.4f\n", m.TotalCostUSD)
    }
}
```

Messages arrive in order, but there's no guarantee about timing. A long
response might arrive as a single `AssistantMessage`, or Claude might send
multiple messages.

## Character-by-Character Streaming

For real-time display (like a typing effect), enable partial messages and
handle stream events:

```go
client, _ := goclaude.NewClient(
    goclaude.WithIncludePartialMessages(true),
)

for msg := range client.Query(ctx, "Write a haiku.") {
    switch m := msg.(type) {
    case goclaude.StreamEvent:
        if m.Event == "delta" {
            fmt.Print(m.Delta) // Print without newline
        }
    case goclaude.ResultMessage:
        fmt.Println() // Final newline
    }
}
```

Stream events with `Event == "delta"` contain partial text. The `"done"` event
signals the end of a streaming chunk.

## Multi-Turn Conversations

Use `Stream` for conversations with multiple exchanges:

```go
stream, _ := client.Stream(ctx)
defer stream.Close()

// First exchange
stream.Send(ctx, "Let's plan a trip to Japan.")
waitForResponse(stream)

// Second exchange (Claude remembers the context)
stream.Send(ctx, "What's the best time to visit Kyoto?")
waitForResponse(stream)

// Third exchange
stream.Send(ctx, "Book me a hotel for cherry blossom season.")
waitForResponse(stream)
```

The helper function:

```go
func waitForResponse(stream *goclaude.Stream) {
    for msg := range stream.Messages() {
        switch m := msg.(type) {
        case goclaude.AssistantMessage:
            fmt.Println(m.ContentText())
        case goclaude.ResultMessage:
            return // Done with this turn
        }
    }
}
```

## Interrupting Generation

Stop Claude mid-generation:

```go
stream, _ := client.Stream(ctx)

stream.Send(ctx, "Write a very long essay about climate change.")

// Start receiving
go func() {
    time.Sleep(2 * time.Second)
    stream.Interrupt(ctx) // Stop after 2 seconds
}()

for msg := range stream.Messages() {
    if m, ok := msg.(goclaude.AssistantMessage); ok {
        fmt.Println(m.ContentText())
    }
}
```

After interruption, you can still send new messages to the stream.

## Message Types

Understanding the message types helps you build robust handlers:

### AssistantMessage

Claude's response. May contain text, tool use requests, or thinking blocks.

```go
case goclaude.AssistantMessage:
    // Get just the text content
    fmt.Println(m.ContentText())

    // Or inspect all content blocks
    for _, block := range m.Message.Content {
        switch block.Type {
        case "text":
            fmt.Println("Text:", block.Text)
        case "tool_use":
            fmt.Printf("Tool: %s, ID: %s\n", block.Name, block.ID)
        case "thinking":
            fmt.Println("Thinking:", block.Text)
        }
    }
```

### ResultMessage

Final status with usage statistics.

```go
case goclaude.ResultMessage:
    fmt.Printf("Status: %s\n", m.Status)
    fmt.Printf("Subtype: %s\n", m.Subtype) // success, error_max_turns, etc.
    fmt.Printf("Cost: $%.4f\n", m.TotalCostUSD)
    fmt.Printf("Duration: %dms\n", m.DurationMs)

    if m.Usage != nil {
        fmt.Printf("Tokens: %d in, %d out\n",
            m.Usage.InputTokens, m.Usage.OutputTokens)
    }

    // Per-model breakdown
    for model, usage := range m.ModelUsage {
        fmt.Printf("%s: $%.4f\n", model, usage.CostUSD)
    }
```

### StreamEvent

Real-time deltas during generation.

```go
case goclaude.StreamEvent:
    switch m.Event {
    case "delta":
        fmt.Print(m.Delta)
    case "done":
        // Chunk complete
    }
```

### TodoUpdateMessage

Task tracking updates from Claude.

```go
case goclaude.TodoUpdateMessage:
    for _, item := range m.Items {
        status := "[ ]"
        if item.Status == goclaude.TodoStatusCompleted {
            status = "[x]"
        } else if item.Status == goclaude.TodoStatusInProgress {
            status = "[~]"
        }
        fmt.Printf("%s %s\n", status, item.Content)
    }
```

### SystemMessage

Initialization and compaction boundaries.

```go
case goclaude.SystemMessage:
    if m.Subtype == "init" {
        fmt.Printf("Model: %s\n", m.Model)
        fmt.Printf("Tools: %v\n", m.Tools)
    }

case goclaude.CompactBoundaryMessage:
    fmt.Printf("Context compacted (trigger: %s, pre-tokens: %d)\n",
        m.CompactMetadata.Trigger, m.CompactMetadata.PreTokens)
```

## Pull-Based Iteration

For more control, convert the iterator to pull-based with `iter.Pull`:

```go
messages := client.Query(ctx, prompt)
next, stop := iter.Pull(messages)
defer stop()

for {
    msg, ok := next()
    if !ok {
        break
    }

    // Process message
    if shouldStop(msg) {
        stop() // Early termination
        break
    }
}
```

This lets you stop iteration based on conditions without goroutines.

## Concurrent Message Processing

Process messages in a separate goroutine:

```go
stream, _ := client.Stream(ctx)

// Producer
go func() {
    stream.Send(ctx, "Analyze this codebase.")
}()

// Consumer with timeout
done := make(chan struct{})
go func() {
    defer close(done)
    for msg := range stream.Messages() {
        handleMessage(msg)
    }
}()

select {
case <-done:
    // Normal completion
case <-time.After(5 * time.Minute):
    stream.Interrupt(ctx)
    <-done // Wait for cleanup
}
```

## Error Handling

Streams don't return errors directly. Check for error messages:

```go
for msg := range client.Query(ctx, prompt) {
    switch m := msg.(type) {
    case goclaude.ResultMessage:
        if m.IsError || m.Subtype != "success" {
            fmt.Printf("Error: %v\n", m.Errors)
        }
    }
}
```

Common error subtypes:
- `error_max_turns` - Exceeded turn limit
- `error_during_execution` - Tool execution failed
- `error_max_budget_usd` - Budget exceeded

## Complete Example: Interactive Chat

```go
package main

import (
    "bufio"
    "context"
    "fmt"
    "os"
    "strings"

    "github.com/roasbeef/goclaude"
)

func main() {
    client, _ := goclaude.NewClient(
        goclaude.WithSystemPrompt("You are a helpful assistant."),
        goclaude.WithIncludePartialMessages(true),
    )
    defer client.Close()

    ctx := context.Background()
    stream, _ := client.Stream(ctx)
    defer stream.Close()

    reader := bufio.NewReader(os.Stdin)
    fmt.Println("Chat with Claude (type 'quit' to exit)")

    for {
        fmt.Print("\nYou: ")
        input, _ := reader.ReadString('\n')
        input = strings.TrimSpace(input)

        if input == "quit" {
            break
        }

        if input == "" {
            continue
        }

        stream.Send(ctx, input)

        fmt.Print("Claude: ")
        for msg := range stream.Messages() {
            switch m := msg.(type) {
            case goclaude.StreamEvent:
                if m.Event == "delta" {
                    fmt.Print(m.Delta)
                }
            case goclaude.AssistantMessage:
                // Already printed via deltas
            case goclaude.ResultMessage:
                fmt.Printf("\n[%.4f USD]\n", m.TotalCostUSD)
                goto nextTurn
            }
        }
    nextTurn:
    }
}
```

## Performance Tips

**Batch processing.** If you need multiple independent queries, run them
concurrently with separate clients:

```go
var wg sync.WaitGroup
for _, prompt := range prompts {
    wg.Add(1)
    go func(p string) {
        defer wg.Done()
        client, _ := goclaude.NewClient()
        defer client.Close()
        for msg := range client.Query(ctx, p) {
            // process
        }
    }(prompt)
}
wg.Wait()
```

**Stream buffer sizing.** The internal channel has limited capacity. If you're
doing heavy processing, consider buffering:

```go
buffered := make(chan goclaude.Message, 100)
go func() {
    for msg := range stream.Messages() {
        buffered <- msg
    }
    close(buffered)
}()

for msg := range buffered {
    // slower processing won't block the stream
}
```

## See Also

- [Sessions](sessions.md) - Persist conversations across restarts
- [Hooks](hooks.md) - Intercept messages before processing
