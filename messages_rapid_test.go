package claudeagent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestParseMarshalRoundtripRapid uses property-based testing to verify
// that all messages can be marshaled to JSON and parsed back.
func TestParseMarshalRoundtripRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random message
		msg := genMessage().Draw(t, "message")

		// Marshal to JSON
		data, err := json.Marshal(msg)
		require.NoError(t, err, "marshal should succeed")

		// Parse back
		parsed, err := ParseMessage(data)
		require.NoError(t, err, "parse should succeed")

		// Verify type matches
		require.Equal(t, msg.MessageType(), parsed.MessageType(),
			"message type should match after roundtrip")
	})
}

// TestUserMessageRoleAlwaysUser verifies that UserMessage.Role is always "user".
func TestUserMessageRoleAlwaysUserRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		msg := genUserMessage().Draw(t, "user_message")

		// Invariant: role is always "user"
		require.Equal(t, "user", msg.Message.Role,
			"UserMessage role must always be 'user'")

		// Invariant: type is always "user"
		require.Equal(t, "user", msg.Type,
			"UserMessage type must always be 'user'")

		// Invariant: message type is always "user"
		require.Equal(t, "user", msg.MessageType(),
			"UserMessage.MessageType() must always return 'user'")
	})
}

// TestAssistantMessageContentTextNeverPanicsRapid verifies that ContentText
// never panics regardless of content structure.
func TestAssistantMessageContentTextNeverPanicsRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		msg := genAssistantMessage().Draw(t, "assistant_message")

		// This should never panic
		require.NotPanics(t, func() {
			_ = msg.ContentText()
		}, "ContentText should never panic")
	})
}

// TestContentTextOnlyTextBlocks verifies that ContentText only includes
// text blocks, not thinking or tool_use blocks.
func TestContentTextOnlyTextBlocksRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		msg := genAssistantMessage().Draw(t, "assistant_message")

		text := msg.ContentText()

		// Invariant: result should only contain text from "text" blocks
		expectedText := ""
		for _, block := range msg.Message.Content {
			if block.Type == "text" {
				expectedText += block.Text
			}
		}

		require.Equal(t, expectedText, text,
			"ContentText should only include text blocks")
	})
}

// TestTodoStatusValidValues verifies that TodoStatus is always one of
// the defined constants.
func TestTodoStatusValidValuesRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		status := genTodoStatus().Draw(t, "todo_status")

		// Invariant: status must be one of the valid values
		validStatuses := map[TodoStatus]bool{
			TodoStatusPending:    true,
			TodoStatusInProgress: true,
			TodoStatusCompleted:  true,
		}

		require.True(t, validStatuses[status],
			"TodoStatus must be a valid constant")
	})
}

// TestControlRequestRequestIDNotEmpty verifies that control requests
// always have non-empty request IDs.
func TestControlRequestRequestIDNotEmptyRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		req := genControlRequest().Draw(t, "control_request")

		// Invariant: request ID must not be empty
		require.NotEmpty(t, req.RequestID,
			"ControlRequest.RequestID must not be empty")

		// Invariant: type must be "control"
		require.Equal(t, "control", req.Type,
			"ControlRequest.Type must be 'control'")
	})
}

// TestPermissionResultIsAllowConsistent verifies that IsAllow returns
// true for PermissionAllow and false for PermissionDeny.
func TestPermissionResultIsAllowConsistentRapid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Test PermissionAllow
		allow := PermissionAllow{}
		require.True(t, allow.IsAllow(),
			"PermissionAllow.IsAllow() must return true")

		// Test PermissionDeny
		reason := rapid.String().Draw(t, "deny_reason")
		deny := PermissionDeny{Reason: reason}
		require.False(t, deny.IsAllow(),
			"PermissionDeny.IsAllow() must return false")
	})
}

// Generators for rapid property-based testing

// genMessage generates arbitrary messages.
func genMessage() *rapid.Generator[Message] {
	return rapid.OneOf(
		rapid.Map(genUserMessage(), func(m UserMessage) Message { return m }),
		rapid.Map(genAssistantMessage(), func(m AssistantMessage) Message { return m }),
		rapid.Map(genResultMessage(), func(m ResultMessage) Message { return m }),
		rapid.Map(genStreamEvent(), func(m StreamEvent) Message { return m }),
		rapid.Map(genTodoUpdateMessage(), func(m TodoUpdateMessage) Message { return m }),
		rapid.Map(genSubagentResultMessage(), func(m SubagentResultMessage) Message { return m }),
		rapid.Map(genControlRequest(), func(m ControlRequest) Message { return m }),
		rapid.Map(genControlResponse(), func(m ControlResponse) Message { return m }),
	)
}

// genUserMessage generates arbitrary user messages.
func genUserMessage() *rapid.Generator[UserMessage] {
	return rapid.Custom(func(t *rapid.T) UserMessage {
		content := rapid.String().Draw(t, "content")
		return UserMessage{
			Type:      "user",
			SessionID: rapid.String().Draw(t, "session_id"),
			Message: APIUserMessage{
				Role: "user",
				Content: []UserContentBlock{
					{Type: "text", Text: content},
				},
			},
		}
	})
}

// genAssistantMessage generates arbitrary assistant messages.
func genAssistantMessage() *rapid.Generator[AssistantMessage] {
	return rapid.Custom(func(t *rapid.T) AssistantMessage {
		msg := AssistantMessage{
			Type: "assistant",
		}
		msg.Message.Role = "assistant"
		msg.Message.Content = rapid.SliceOf(genContentBlock()).
			Draw(t, "content_blocks")

		// Optionally add usage
		if rapid.Bool().Draw(t, "has_usage") {
			msg.Usage = &Usage{
				InputTokens:  rapid.IntRange(0, 10000).Draw(t, "input_tokens"),
				OutputTokens: rapid.IntRange(0, 10000).Draw(t, "output_tokens"),
			}
			msg.Usage.TotalTokens = msg.Usage.InputTokens + msg.Usage.OutputTokens
			msg.Usage.Cost = float64(msg.Usage.TotalTokens) * 0.00001
		}

		return msg
	})
}

// genContentBlock generates arbitrary content blocks.
func genContentBlock() *rapid.Generator[ContentBlock] {
	return rapid.OneOf(
		// Text block
		rapid.Custom(func(t *rapid.T) ContentBlock {
			return ContentBlock{
				Type: "text",
				Text: rapid.String().Draw(t, "text"),
			}
		}),
		// Thinking block
		rapid.Custom(func(t *rapid.T) ContentBlock {
			return ContentBlock{
				Type: "thinking",
				Text: rapid.String().Draw(t, "thinking"),
			}
		}),
		// Tool use block
		rapid.Custom(func(t *rapid.T) ContentBlock {
			args := map[string]interface{}{
				"arg1": rapid.String().Draw(t, "arg1"),
			}
			argsJSON, _ := json.Marshal(args)
			return ContentBlock{
				Type:  "tool_use",
				ID:    rapid.String().Draw(t, "tool_id"),
				Name:  rapid.String().Draw(t, "tool_name"),
				Input: argsJSON,
			}
		}),
	)
}

// genResultMessage generates arbitrary result messages.
func genResultMessage() *rapid.Generator[ResultMessage] {
	return rapid.Custom(func(t *rapid.T) ResultMessage {
		status := rapid.SampledFrom([]string{"success", "error"}).
			Draw(t, "status")
		return ResultMessage{
			Type:   "result",
			Status: status,
			Result: rapid.String().Draw(t, "result"),
		}
	})
}

// genStreamEvent generates arbitrary stream events.
func genStreamEvent() *rapid.Generator[StreamEvent] {
	return rapid.Custom(func(t *rapid.T) StreamEvent {
		event := rapid.SampledFrom([]string{"delta", "done"}).
			Draw(t, "event")
		// Generate timestamp from Unix seconds
		unixSec := rapid.Int64Range(0, 2000000000).Draw(t, "unix_sec")
		return StreamEvent{
			Type:      "stream_event",
			Event:     event,
			Delta:     rapid.String().Draw(t, "delta"),
			Timestamp: time.Unix(unixSec, 0),
		}
	})
}

// genTodoUpdateMessage generates arbitrary todo update messages.
func genTodoUpdateMessage() *rapid.Generator[TodoUpdateMessage] {
	return rapid.Custom(func(t *rapid.T) TodoUpdateMessage {
		return TodoUpdateMessage{
			Type:  "todo_update",
			Items: rapid.SliceOf(genTodoItem()).Draw(t, "items"),
		}
	})
}

// genTodoItem generates arbitrary todo items.
func genTodoItem() *rapid.Generator[TodoItem] {
	return rapid.Custom(func(t *rapid.T) TodoItem {
		return TodoItem{
			Content:    rapid.String().Draw(t, "content"),
			ActiveForm: rapid.String().Draw(t, "active_form"),
			Status:     genTodoStatus().Draw(t, "status"),
		}
	})
}

// genTodoStatus generates arbitrary todo statuses.
func genTodoStatus() *rapid.Generator[TodoStatus] {
	return rapid.SampledFrom([]TodoStatus{
		TodoStatusPending,
		TodoStatusInProgress,
		TodoStatusCompleted,
	})
}

// genSubagentResultMessage generates arbitrary subagent result messages.
func genSubagentResultMessage() *rapid.Generator[SubagentResultMessage] {
	return rapid.Custom(func(t *rapid.T) SubagentResultMessage {
		return SubagentResultMessage{
			Type:      "subagent_result",
			AgentName: rapid.String().Draw(t, "agent_name"),
			Status:    rapid.SampledFrom([]string{"success", "error"}).Draw(t, "status"),
			Result:    rapid.String().Draw(t, "result"),
		}
	})
}

// genControlRequest generates arbitrary control requests.
func genControlRequest() *rapid.Generator[ControlRequest] {
	return rapid.Custom(func(t *rapid.T) ControlRequest {
		return ControlRequest{
			Type:      "control",
			Subtype:   rapid.String().Draw(t, "subtype"),
			RequestID: rapid.StringMatching(`req_[0-9]+`).Draw(t, "request_id"),
			Payload:   make(map[string]interface{}),
		}
	})
}

// genControlResponse generates arbitrary control responses.
func genControlResponse() *rapid.Generator[ControlResponse] {
	return rapid.Custom(func(t *rapid.T) ControlResponse {
		resp := ControlResponse{
			Type:      "control",
			RequestID: rapid.StringMatching(`req_[0-9]+`).Draw(t, "request_id"),
		}

		// Either result or error, not both
		if rapid.Bool().Draw(t, "has_error") {
			resp.Error = &ProtocolError{
				Code:    rapid.String().Draw(t, "error_code"),
				Message: rapid.String().Draw(t, "error_message"),
			}
		} else {
			resp.Result = make(map[string]interface{})
		}

		return resp
	})
}
