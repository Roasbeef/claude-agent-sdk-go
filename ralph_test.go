package claudeagent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewRalphLoop tests RalphLoop creation with defaults.
func TestNewRalphLoop(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		loop := NewRalphLoop(RalphConfig{
			Task: "Build a feature",
		})

		cfg := loop.Config()
		assert.Equal(t, "Build a feature", cfg.Task)
		assert.Equal(t, "TASK COMPLETE", cfg.CompletionPromise)
		assert.Equal(t, 10, cfg.MaxIterations)
	})

	t.Run("custom values", func(t *testing.T) {
		loop := NewRalphLoop(RalphConfig{
			Task:              "Implement Redis cache",
			CompletionPromise: "RALPH_DONE",
			MaxIterations:     5,
		})

		cfg := loop.Config()
		assert.Equal(t, "Implement Redis cache", cfg.Task)
		assert.Equal(t, "RALPH_DONE", cfg.CompletionPromise)
		assert.Equal(t, 5, cfg.MaxIterations)
	})
}

// TestRalphLoopBuildPrompt tests prompt construction for iterations.
func TestRalphLoopBuildPrompt(t *testing.T) {
	loop := NewRalphLoop(RalphConfig{
		Task:              "Build a REST API",
		CompletionPromise: "TASK_DONE",
		MaxIterations:     10,
	})

	t.Run("first iteration", func(t *testing.T) {
		prompt := loop.buildPrompt(1)

		// Should include the task.
		assert.Contains(t, prompt, "Build a REST API")
		// Should include completion instructions.
		assert.Contains(t, prompt, "<promise>TASK_DONE</promise>")
		// Should NOT include iteration context (first iteration).
		assert.NotContains(t, prompt, "[Ralph Loop")
	})

	t.Run("subsequent iteration", func(t *testing.T) {
		prompt := loop.buildPrompt(3)

		// Should include iteration context.
		assert.Contains(t, prompt, "[Ralph Loop - Iteration 3/10]")
		// Should include the task.
		assert.Contains(t, prompt, "Task: Build a REST API")
		// Should include completion instructions.
		assert.Contains(t, prompt, "<promise>TASK_DONE</promise>")
		// Should mention previous work.
		assert.Contains(t, prompt, "Your previous work is visible")
	})

	t.Run("final iteration", func(t *testing.T) {
		prompt := loop.buildPrompt(10)

		// Should show correct iteration count.
		assert.Contains(t, prompt, "[Ralph Loop - Iteration 10/10]")
	})
}

// TestRalphLoopState tests the state accessor methods.
func TestRalphLoopState(t *testing.T) {
	loop := NewRalphLoop(RalphConfig{
		Task: "Test task",
	})

	// Initial state.
	assert.False(t, loop.IsComplete())
	assert.Equal(t, 0, loop.CurrentIteration())
	assert.Equal(t, 0.0, loop.TotalCost())

	// Simulate state changes.
	loop.mu.Lock()
	loop.iteration = 3
	loop.complete = true
	loop.totalCost = 0.05
	loop.mu.Unlock()

	assert.True(t, loop.IsComplete())
	assert.Equal(t, 3, loop.CurrentIteration())
	assert.Equal(t, 0.05, loop.TotalCost())
}

// TestIteration tests the Iteration struct.
func TestIteration(t *testing.T) {
	iter := &Iteration{
		Number:       2,
		Messages:     []Message{},
		Complete:     true,
		Error:        nil,
		SessionID:    "session_123",
		CostUSD:      0.01,
		TotalCostUSD: 0.03,
	}

	assert.Equal(t, 2, iter.Number)
	assert.True(t, iter.Complete)
	assert.Equal(t, "session_123", iter.SessionID)
	assert.Equal(t, 0.01, iter.CostUSD)
	assert.Equal(t, 0.03, iter.TotalCostUSD)
}

// TestHookResultStopFields tests the new Stop hook fields in HookResult.
func TestHookResultStopFields(t *testing.T) {
	t.Run("approve decision allows exit", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Decision: "approve",
		}

		assert.True(t, result.Continue)
		assert.Equal(t, "approve", result.Decision)
	})

	t.Run("block decision with reason", func(t *testing.T) {
		result := HookResult{
			Continue:      false,
			Decision:      "block",
			Reason:        "Continue working on the task",
			SystemMessage: "Ralph Loop: Iteration 2 of 10",
		}

		assert.False(t, result.Continue)
		assert.Equal(t, "block", result.Decision)
		assert.Equal(t, "Continue working on the task", result.Reason)
		assert.Equal(t, "Ralph Loop: Iteration 2 of 10", result.SystemMessage)
	})
}

// TestBuildHookResponse tests the hook response builder.
func TestBuildHookResponse(t *testing.T) {
	t.Run("basic continue", func(t *testing.T) {
		result := HookResult{Continue: true}
		resp := buildHookResponse("PostToolUse", result)

		assert.Equal(t, true, resp["continue"])
		_, hasDecision := resp["decision"]
		assert.False(t, hasDecision)
	})

	t.Run("with modify", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Modify:   map[string]interface{}{"key": "value"},
		}
		resp := buildHookResponse("PostToolUse", result)

		assert.Equal(t, true, resp["continue"])
		modify, ok := resp["modify"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "value", modify["key"])
	})

	t.Run("with Stop hook fields", func(t *testing.T) {
		result := HookResult{
			Decision:      "block",
			Reason:        "New prompt here",
			SystemMessage: "Status message",
		}
		resp := buildHookResponse("Stop", result)

		// When Decision is set, continue must be omitted to
		// match shell hook behavior. Shell hooks only output
		// {"decision":"block","reason":"..."} without continue.
		_, hasContinue := resp["continue"]
		assert.False(t, hasContinue,
			"stop hook must not include continue field",
		)
		assert.Equal(t, "block", resp["decision"])
		assert.Equal(t, "New prompt here", resp["reason"])
		assert.Equal(t, "Status message", resp["systemMessage"])
	})

	t.Run("empty strings not included", func(t *testing.T) {
		result := HookResult{
			Continue: true,
			Decision: "", // Empty - should not be included.
		}
		resp := buildHookResponse("PostToolUse", result)

		_, hasDecision := resp["decision"]
		assert.False(t, hasDecision)
		_, hasReason := resp["reason"]
		assert.False(t, hasReason)
		_, hasSystemMessage := resp["systemMessage"]
		assert.False(t, hasSystemMessage)
	})
}

// TestCompletionPromiseDetection tests that the promise tag is correctly detected.
func TestCompletionPromiseDetection(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		promise  string
		expected bool
	}{
		{
			name:     "exact match",
			text:     "I've finished. <promise>TASK COMPLETE</promise>",
			promise:  "TASK COMPLETE",
			expected: true,
		},
		{
			name:     "custom promise",
			text:     "Done! <promise>RALPH_DONE</promise>",
			promise:  "RALPH_DONE",
			expected: true,
		},
		{
			name:     "promise in middle of text",
			text:     "Here is the result <promise>DONE</promise> and more text",
			promise:  "DONE",
			expected: true,
		},
		{
			name:     "no promise tag",
			text:     "I completed the task successfully",
			promise:  "TASK COMPLETE",
			expected: false,
		},
		{
			name:     "wrong promise text",
			text:     "<promise>DIFFERENT</promise>",
			promise:  "TASK COMPLETE",
			expected: false,
		},
		{
			name:     "malformed tag",
			text:     "<promise>TASK COMPLETE",
			promise:  "TASK COMPLETE",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			promiseTag := "<promise>" + tc.promise + "</promise>"
			found := strings.Contains(tc.text, promiseTag)
			assert.Equal(t, tc.expected, found)
		})
	}
}

// TestRalphConfig tests the RalphConfig struct.
func TestRalphConfig(t *testing.T) {
	cfg := RalphConfig{
		Task:              "Build a feature",
		CompletionPromise: "CUSTOM_PROMISE",
		MaxIterations:     15,
	}

	assert.Equal(t, "Build a feature", cfg.Task)
	assert.Equal(t, "CUSTOM_PROMISE", cfg.CompletionPromise)
	assert.Equal(t, 15, cfg.MaxIterations)
}
