// Demo program for Claude Agent SDK.
//
// This demonstrates basic usage of the Go SDK for Claude Code.
// Requires CLAUDE_CODE_OAUTH_TOKEN or ANTHROPIC_API_KEY environment variable.
//
// Usage:
//
//	go run ./cmd/demo "What is 2+2?"
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	claudeagent "github.com/roasbeef/claude-agent-sdk-go"
)

func main() {
	// Check for authentication.
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "Error: CLAUDE_CODE_OAUTH_TOKEN or ANTHROPIC_API_KEY must be set")
		os.Exit(1)
	}

	// Get prompt from command line or use default.
	prompt := "What is 2+2? Answer briefly."
	if len(os.Args) > 1 {
		prompt = strings.Join(os.Args[1:], " ")
	}

	fmt.Printf("Prompt: %s\n\n", prompt)

	// Create client.
	client, err := claudeagent.NewClient(
		claudeagent.WithSystemPrompt("You are a helpful assistant. Keep responses brief and to the point."),
		claudeagent.WithModel("claude-sonnet-4-5-20250929"),
		claudeagent.WithPermissionMode(claudeagent.PermissionModeDefault),
		claudeagent.WithSkillsDisabled(), // Disable skills for simple demo
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Create context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Query Claude and iterate over responses.
	fmt.Println("Response:")
	fmt.Println("─────────")

	for msg := range client.Query(ctx, prompt) {
		switch m := msg.(type) {
		case claudeagent.AssistantMessage:
			// Print assistant text.
			text := m.ContentText()
			if text != "" {
				fmt.Print(text)
			}

		case claudeagent.StreamEvent:
			// Print streaming deltas.
			if m.Event == "delta" && m.Delta != "" {
				fmt.Print(m.Delta)
			}

		case claudeagent.ResultMessage:
			// Print final status.
			fmt.Println()
			fmt.Println("─────────")
			fmt.Printf("Status: %s\n", m.Status)
			if m.Usage != nil {
				fmt.Printf("Tokens: %d input, %d output (cost: $%.4f)\n",
					m.Usage.InputTokens,
					m.Usage.OutputTokens,
					m.TotalCostUSD,
				)
			}

		case claudeagent.TodoUpdateMessage:
			// Print todo updates if any.
			for _, item := range m.Items {
				fmt.Printf("[%s] %s\n", item.Status, item.Content)
			}
		}
	}
}
