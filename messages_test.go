package claudeagent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseMessageUserMessage tests parsing user messages.
func TestParseMessageUserMessage(t *testing.T) {
	input := `{
		"type": "user",
		"message": {
			"role": "user",
			"content": [{"type": "text", "text": "What's the weather?"}]
		},
		"session_id": "sess_123",
		"parent_tool_use_id": null
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)
	require.NotNil(t, msg)

	userMsg, ok := msg.(UserMessage)
	require.True(t, ok, "expected UserMessage")

	assert.Equal(t, "user", userMsg.MessageType())
	assert.Equal(t, "user", userMsg.Message.Role)
	require.Len(t, userMsg.Message.Content, 1)
	assert.Equal(t, "text", userMsg.Message.Content[0].Type)
	assert.Equal(t, "What's the weather?", userMsg.Message.Content[0].Text)
	assert.Equal(t, "sess_123", userMsg.SessionID)
}

// TestParseMessageAssistantMessage tests parsing assistant messages.
func TestParseMessageAssistantMessage(t *testing.T) {
	input := `{
		"type": "assistant",
		"message": {
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "The weather is sunny."
				},
				{
					"type": "thinking",
					"text": "Let me check the current conditions..."
				}
			]
		},
		"usage": {
			"input_tokens": 100,
			"output_tokens": 50,
			"total_tokens": 150,
			"cost": 0.0025
		}
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	assistantMsg, ok := msg.(AssistantMessage)
	require.True(t, ok, "expected AssistantMessage")

	assert.Equal(t, "assistant", assistantMsg.MessageType())
	assert.Equal(t, "assistant", assistantMsg.Message.Role)
	assert.Len(t, assistantMsg.Message.Content, 2)

	// Check text block
	textBlock := assistantMsg.Message.Content[0]
	assert.Equal(t, "text", textBlock.Type)
	assert.Equal(t, "The weather is sunny.", textBlock.Text)

	// Check thinking block
	thinkingBlock := assistantMsg.Message.Content[1]
	assert.Equal(t, "thinking", thinkingBlock.Type)
	assert.Equal(t, "Let me check the current conditions...", thinkingBlock.Text)

	// Check usage
	require.NotNil(t, assistantMsg.Usage)
	assert.Equal(t, 100, assistantMsg.Usage.InputTokens)
	assert.Equal(t, 50, assistantMsg.Usage.OutputTokens)
	assert.Equal(t, 150, assistantMsg.Usage.TotalTokens)
	assert.Equal(t, 0.0025, assistantMsg.Usage.Cost)
}

// TestParseMessageToolUseBlock tests parsing tool use requests.
func TestParseMessageToolUseBlock(t *testing.T) {
	inputJSON := `{
		"type": "assistant",
		"message": {
			"role": "assistant",
			"content": [
				{
					"type": "text",
					"text": "Let me fetch the quote for AAPL."
				},
				{
					"type": "tool_use",
					"id": "toolu_123",
					"name": "fetch_quote",
					"input": {"symbol": "AAPL"}
				}
			]
		}
	}`

	msg, err := ParseMessage([]byte(inputJSON))
	require.NoError(t, err)

	assistantMsg, ok := msg.(AssistantMessage)
	require.True(t, ok)

	require.Len(t, assistantMsg.Message.Content, 2)

	// Check tool use block
	toolBlock := assistantMsg.Message.Content[1]
	assert.Equal(t, "tool_use", toolBlock.Type)
	assert.Equal(t, "toolu_123", toolBlock.ID)
	assert.Equal(t, "fetch_quote", toolBlock.Name)

	// Parse input
	var toolInput map[string]interface{}
	err = json.Unmarshal(toolBlock.Input, &toolInput)
	require.NoError(t, err)
	assert.Equal(t, "AAPL", toolInput["symbol"])
}

// TestParseMessageResultMessage tests parsing result messages.
func TestParseMessageResultMessage(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedStatus string
		expectedResult string
	}{
		{
			name: "success result",
			input: `{
				"type": "result",
				"status": "success",
				"result": "Query completed successfully"
			}`,
			expectedStatus: "success",
			expectedResult: "Query completed successfully",
		},
		{
			name: "error result",
			input: `{
				"type": "result",
				"status": "error",
				"result": "API key not found"
			}`,
			expectedStatus: "error",
			expectedResult: "API key not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			resultMsg, ok := msg.(ResultMessage)
			require.True(t, ok)

			assert.Equal(t, "result", resultMsg.MessageType())
			assert.Equal(t, tt.expectedStatus, resultMsg.Status)
			assert.Equal(t, tt.expectedResult, resultMsg.Result)
		})
	}
}

// TestParseMessageStreamEvent tests parsing stream events.
func TestParseMessageStreamEvent(t *testing.T) {
	input := `{
		"type": "stream_event",
		"event": "delta",
		"delta": "The ",
		"timestamp": "2025-10-22T10:30:00Z"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	streamEvent, ok := msg.(StreamEvent)
	require.True(t, ok)

	assert.Equal(t, "stream_event", streamEvent.MessageType())
	assert.Equal(t, "delta", streamEvent.Event)
	assert.Equal(t, "The ", streamEvent.Delta)
	assert.False(t, streamEvent.Timestamp.IsZero())
}

// TestParseMessageTodoUpdate tests parsing todo updates.
func TestParseMessageTodoUpdate(t *testing.T) {
	input := `{
		"type": "todo_update",
		"items": [
			{
				"content": "Run tests",
				"activeForm": "Running tests",
				"status": "in_progress"
			},
			{
				"content": "Build project",
				"activeForm": "Building project",
				"status": "pending"
			},
			{
				"content": "Deploy to staging",
				"activeForm": "Deploying to staging",
				"status": "completed"
			}
		]
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	todoMsg, ok := msg.(TodoUpdateMessage)
	require.True(t, ok)

	assert.Equal(t, "todo_update", todoMsg.MessageType())
	assert.Len(t, todoMsg.Items, 3)

	// Check in_progress item
	item0 := todoMsg.Items[0]
	assert.Equal(t, "Run tests", item0.Content)
	assert.Equal(t, "Running tests", item0.ActiveForm)
	assert.Equal(t, TodoStatusInProgress, item0.Status)

	// Check pending item
	item1 := todoMsg.Items[1]
	assert.Equal(t, TodoStatusPending, item1.Status)

	// Check completed item
	item2 := todoMsg.Items[2]
	assert.Equal(t, TodoStatusCompleted, item2.Status)
}

// TestParseMessageSubagentResult tests parsing subagent results.
func TestParseMessageSubagentResult(t *testing.T) {
	input := `{
		"type": "subagent_result",
		"agent_name": "research",
		"status": "success",
		"result": "Analysis complete: AAPL shows strong fundamentals"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	subagentMsg, ok := msg.(SubagentResultMessage)
	require.True(t, ok)

	assert.Equal(t, "subagent_result", subagentMsg.MessageType())
	assert.Equal(t, "research", subagentMsg.AgentName)
	assert.Equal(t, "success", subagentMsg.Status)
	assert.Contains(t, subagentMsg.Result, "strong fundamentals")
}

// TestParseMessageControlRequest tests parsing control requests.
func TestParseMessageControlRequest(t *testing.T) {
	input := `{
		"type": "control",
		"subtype": "initialize",
		"requestId": "req_1",
		"payload": {
			"hooks": {
				"PreToolUse": [{"matcher": "*"}]
			}
		}
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	ctrlReq, ok := msg.(ControlRequest)
	require.True(t, ok)

	assert.Equal(t, "control", ctrlReq.MessageType())
	assert.Equal(t, "initialize", ctrlReq.Subtype)
	assert.Equal(t, "req_1", ctrlReq.RequestID)
	assert.NotNil(t, ctrlReq.Payload)
}

// TestParseMessageControlResponse tests parsing control responses.
func TestParseMessageControlResponse(t *testing.T) {
	t.Run("success response", func(t *testing.T) {
		input := `{
			"type": "control",
			"requestId": "req_1",
			"result": {
				"status": "ok"
			}
		}`

		msg, err := ParseMessage([]byte(input))
		require.NoError(t, err)

		ctrlResp, ok := msg.(ControlResponse)
		require.True(t, ok)

		assert.Equal(t, "control", ctrlResp.MessageType())
		assert.Equal(t, "req_1", ctrlResp.RequestID)
		assert.NotNil(t, ctrlResp.Result)
		assert.Nil(t, ctrlResp.Error)
	})

	t.Run("error response", func(t *testing.T) {
		input := `{
			"type": "control",
			"requestId": "req_2",
			"error": {
				"code": "permission_denied",
				"message": "Tool execution denied"
			}
		}`

		msg, err := ParseMessage([]byte(input))
		require.NoError(t, err)

		ctrlResp, ok := msg.(ControlResponse)
		require.True(t, ok)

		require.NotNil(t, ctrlResp.Error)
		assert.Equal(t, "permission_denied", ctrlResp.Error.Code)
		assert.Equal(t, "Tool execution denied", ctrlResp.Error.Message)
	})
}

// TestParseMessageUnknownType tests handling of unknown message types.
func TestParseMessageUnknownType(t *testing.T) {
	input := `{
		"type": "unknown_type",
		"data": "something"
	}`

	msg, err := ParseMessage([]byte(input))
	assert.Error(t, err)
	assert.Nil(t, msg)

	var unknownErr *ErrUnknownMessageType
	assert.ErrorAs(t, err, &unknownErr)
	assert.Equal(t, "unknown_type", unknownErr.Type)
}

// TestParseMessageInvalidJSON tests handling of invalid JSON.
func TestParseMessageInvalidJSON(t *testing.T) {
	input := `{invalid json`

	msg, err := ParseMessage([]byte(input))
	assert.Error(t, err)
	assert.Nil(t, msg)
}

// TestAssistantMessageContentText tests content text extraction.
func TestAssistantMessageContentText(t *testing.T) {
	msg := AssistantMessage{
		Type: "assistant",
	}
	msg.Message.Content = []ContentBlock{
		{Type: "text", Text: "Hello "},
		{Type: "thinking", Text: "What should I say next?"},
		{Type: "text", Text: "world!"},
		{Type: "tool_use", ID: "toolu_123", Name: "some_tool"},
	}

	text := msg.ContentText()
	assert.Equal(t, "Hello world!", text)
}

// TestMessageRoundtrip tests serialization and parsing roundtrip.
func TestMessageRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "user message",
			msg: UserMessage{
				Type:      "user",
				SessionID: "sess_1",
			},
		},
		{
			name: "result message",
			msg: ResultMessage{
				Type:   "result",
				Status: "success",
				Result: "All done",
			},
		},
		{
			name: "stream event",
			msg: StreamEvent{
				Type:      "stream_event",
				Event:     "delta",
				Delta:     "text",
				Timestamp: time.Now(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Serialize
			data, err := json.Marshal(tt.msg)
			require.NoError(t, err)

			// Parse
			parsed, err := ParseMessage(data)
			require.NoError(t, err)

			// Check type matches
			assert.Equal(t, tt.msg.MessageType(), parsed.MessageType())
		})
	}
}

// BenchmarkParseMessage benchmarks message parsing performance.
func BenchmarkParseMessage(b *testing.B) {
	input := []byte(`{
		"type": "assistant",
		"message": {
			"role": "assistant",
			"content": [
				{"type": "text", "text": "Hello world"}
			]
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseMessage(input)
	}
}

// BenchmarkContentText benchmarks content text extraction.
func BenchmarkContentText(b *testing.B) {
	msg := AssistantMessage{
		Type: "assistant",
	}
	msg.Message.Content = []ContentBlock{
		{Type: "text", Text: "Hello "},
		{Type: "text", Text: "world!"},
		{Type: "tool_use", Name: "tool"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.ContentText()
	}
}
