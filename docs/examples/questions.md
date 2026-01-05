# Handling Questions

When Claude needs clarification, it can ask interactive questions using the
`AskUserQuestion` tool. In the CLI, these appear as multiple-choice prompts
navigated with arrow keys. The SDK lets you handle these programmatically.

## Question Structure

Claude can ask 1-4 questions at once, each with optional multiple-choice
options:

```go
type QuestionItem struct {
    Question    string           // The question text
    Header      string           // Short label (max 12 chars)
    Options     []QuestionOption // Available choices
    MultiSelect bool             // Allow multiple selections
}

type QuestionOption struct {
    Label       string // Display text
    Description string // Explanation of this choice
}
```

## Unified Query API (Recommended)

Questions appear in the normal message stream as `QuestionMessage`:

```go
for msg := range client.Query(ctx, "Help me set up the project") {
    switch m := msg.(type) {
    case claudeagent.QuestionMessage:
        // Claude is asking a question
        fmt.Println("Claude asks:", m.Questions[0].Question)

        for i, opt := range m.Questions[0].Options {
            fmt.Printf("  %d. %s - %s\n", i+1, opt.Label, opt.Description)
        }

        // Respond with the first option
        m.Respond(m.AnswerAll(m.Q(0).SelectIndex(0)))

    case claudeagent.AssistantMessage:
        fmt.Println(m.ContentText())

    case claudeagent.ResultMessage:
        fmt.Println("Done:", m.Result)
    }
}
```

The `QuestionMessage` type embeds `QuestionSet`, so all helper methods are
available directly on the message.

## Callback API

For automatic question handling without message inspection:

```go
client, _ := claudeagent.NewClient(
    claudeagent.WithAskUserQuestionHandler(
        func(ctx context.Context, qs claudeagent.QuestionSet) (claudeagent.Answers, error) {
            // Log the question
            log.Printf("Claude asks: %s", qs.Questions[0].Question)

            // Auto-select first option
            return qs.AnswerAll(qs.Q(0).SelectIndex(0)), nil
        },
    ),
)
```

When a handler is configured, `QuestionMessage` won't appear in the `Query()`
stream - questions are handled automatically.

## Fluent Answer Helpers

The SDK provides helpers to avoid error-prone raw map construction:

### Select by Label

```go
// Select an option by its display text
m.Q(0).Select("PostgreSQL")
```

### Select by Index

```go
// Select the first option (0-indexed)
m.Q(0).SelectIndex(0)
```

### Multiple Selection

For questions with `MultiSelect: true`:

```go
// Select multiple options
m.Q(0).SelectMultiple("Feature A", "Feature B", "Feature C")
```

### Freeform Text

For questions without options or to provide custom input:

```go
// Provide freeform text response
m.Q(0).Text("My custom answer")
```

### Combining Multiple Answers

When Claude asks multiple questions at once:

```go
m.Respond(m.AnswerAll(
    m.Q(0).Select("PostgreSQL"),      // First question: select by label
    m.Q(1).SelectIndex(0),            // Second question: select first option
    m.Q(2).Text("Using defaults"),    // Third question: freeform text
))
```

### Simple Single Answer

For the common case of answering one question:

```go
m.Respond(m.Answer(0, "Yes"))
```

## Iterator API

For more control over the question/answer flow:

```go
for qs, answer := range client.Questions(ctx, "Configure the project") {
    // Display questions
    for i, q := range qs.Questions {
        fmt.Printf("Q%d: %s\n", i+1, q.Question)
        for j, opt := range q.Options {
            fmt.Printf("  %d. %s\n", j+1, opt.Label)
        }
    }

    // Collect answers and respond
    answer(qs.AnswerAll(
        qs.Q(0).SelectIndex(getUserChoice()),
    ))
}
```

The iterator yields `QuestionSet` and `AnswerFunc` pairs, giving you explicit
control over when answers are sent.

## Complete Example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/roasbeef/claudeagent"
)

func main() {
    client, err := claudeagent.NewClient(
        claudeagent.WithSystemPrompt("You are a helpful setup assistant."),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()

    for msg := range client.Query(ctx, "Help me initialize a new Go project") {
        switch m := msg.(type) {
        case claudeagent.QuestionMessage:
            handleQuestion(m)

        case claudeagent.AssistantMessage:
            fmt.Println(m.ContentText())

        case claudeagent.ResultMessage:
            if m.IsError {
                log.Printf("Error: %s", m.Error)
            }
        }
    }
}

func handleQuestion(m claudeagent.QuestionMessage) {
    fmt.Println("\n--- Claude has a question ---")

    for i, q := range m.Questions {
        fmt.Printf("\n%s\n", q.Question)

        if len(q.Options) > 0 {
            for j, opt := range q.Options {
                fmt.Printf("  %d. %s", j+1, opt.Label)
                if opt.Description != "" {
                    fmt.Printf(" - %s", opt.Description)
                }
                fmt.Println()
            }
        }

        // For this example, always select the first option or provide default text
        if len(q.Options) > 0 {
            fmt.Printf("  -> Selecting: %s\n", q.Options[0].Label)
        } else {
            fmt.Printf("  -> Responding with default\n")
        }
    }

    // Build answers for all questions
    var answers []claudeagent.QuestionAnswer
    for i, q := range m.Questions {
        if len(q.Options) > 0 {
            answers = append(answers, m.Q(i).SelectIndex(0))
        } else {
            answers = append(answers, m.Q(i).Text("default"))
        }
    }

    if err := m.Respond(m.AnswerAll(answers...)); err != nil {
        log.Printf("Failed to respond: %v", err)
    }
}
```

## Error Handling

The SDK defines specific error types for question handling:

```go
// Question not found (e.g., trying to respond to an expired question)
var notFound *claudeagent.ErrQuestionNotFound
if errors.As(err, &notFound) {
    log.Printf("Question %s not found", notFound.ToolUseID)
}

// Question timed out waiting for response
var timeout *claudeagent.ErrQuestionTimeout
if errors.As(err, &timeout) {
    log.Printf("Question %s timed out", timeout.ToolUseID)
}
```

## See Also

- [Streaming](streaming.md) - Understanding the message stream
- [Hooks](hooks.md) - Intercepting tool calls including AskUserQuestion
- [Sessions](sessions.md) - Managing conversation state
