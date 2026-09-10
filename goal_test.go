package claudeagent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoalStatusMessageJSONRoundTripMet covers the met=true wire form,
// including the cumulative duration/token counters and the reason field.
func TestGoalStatusMessageJSONRoundTripMet(t *testing.T) {
	original := GoalStatusMessage{
		Type:       "system",
		Subtype:    "goal_status",
		Met:        true,
		Condition:  "all tests pass",
		Reason:     "test suite reported zero failures",
		Iterations: 5,
		DurationMs: 12345,
		Tokens:     6789,
		UUID:       "550e8400-e29b-41d4-a716-446655440300",
		SessionID:  "sess_goal_001",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded GoalStatusMessage
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original, decoded)
	assert.True(t, decoded.Met)
	assert.Equal(t, "system", decoded.MessageType())
}

// TestGoalStatusMessageJSONRoundTripNotMet covers the met=false form and
// asserts that the empty optional fields are omitted on the wire.
func TestGoalStatusMessageJSONRoundTripNotMet(t *testing.T) {
	original := GoalStatusMessage{
		Type:      "system",
		Subtype:   "goal_status",
		Met:       false,
		Condition: "all tests pass",
		UUID:      "550e8400-e29b-41d4-a716-446655440301",
		SessionID: "sess_goal_002",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Optional fields with zero values should be omitted.
	assert.NotContains(t, string(data), `"reason"`)
	assert.NotContains(t, string(data), `"iterations"`)
	assert.NotContains(t, string(data), `"durationMs"`)
	assert.NotContains(t, string(data), `"tokens"`)

	var decoded GoalStatusMessage
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original, decoded)
	assert.False(t, decoded.Met)
	assert.Empty(t, decoded.Reason)
	assert.Zero(t, decoded.Iterations)
	assert.Zero(t, decoded.DurationMs)
	assert.Zero(t, decoded.Tokens)
}

// TestGoalStatusMessageType ensures the message identifies as "system" so it
// routes through the system-message handling path on consumers.
func TestGoalStatusMessageType(t *testing.T) {
	var msg GoalStatusMessage
	assert.Equal(t, "system", msg.MessageType())
}

// TestParseMessageGoalStatus drives the parser dispatch path: a system
// envelope with subtype "goal_status" must materialize as GoalStatusMessage.
func TestParseMessageGoalStatus(t *testing.T) {
	input := `{
		"type": "system",
		"subtype": "goal_status",
		"met": true,
		"condition": "all tests pass",
		"reason": "test suite reported zero failures",
		"iterations": 5,
		"durationMs": 12345,
		"tokens": 6789,
		"uuid": "550e8400-e29b-41d4-a716-446655440302",
		"session_id": "sess_goal_003"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	goalMsg, ok := msg.(GoalStatusMessage)
	require.True(t, ok, "expected GoalStatusMessage")

	assert.Equal(t, "system", goalMsg.MessageType())
	assert.Equal(t, "goal_status", goalMsg.Subtype)
	assert.True(t, goalMsg.Met)
	assert.Equal(t, "all tests pass", goalMsg.Condition)
	assert.Equal(t, "test suite reported zero failures", goalMsg.Reason)
	assert.Equal(t, 5, goalMsg.Iterations)
	assert.Equal(t, int64(12345), goalMsg.DurationMs)
	assert.Equal(t, 6789, goalMsg.Tokens)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440302", goalMsg.UUID)
	assert.Equal(t, "sess_goal_003", goalMsg.SessionID)
}

// TestWithGoalSetsCondition asserts WithGoal stores the condition on Options
// verbatim when within the length cap.
func TestWithGoalSetsCondition(t *testing.T) {
	opts := DefaultOptions()
	WithGoal("all tests pass")(&opts)
	assert.Equal(t, "all tests pass", opts.Goal)
}

// TestWithGoalTruncates verifies that conditions over 1000 characters are
// truncated to the cap so the CLI flag stays within accepted bounds.
func TestWithGoalTruncates(t *testing.T) {
	long := strings.Repeat("a", 1500)
	opts := DefaultOptions()
	WithGoal(long)(&opts)
	assert.Len(t, opts.Goal, 1000)
	assert.Equal(t, strings.Repeat("a", 1000), opts.Goal)
}

// TestErrGoalAchievedError exercises the error formatting so callers
// type-switching on the error get a useful Error() string.
func TestErrGoalAchievedError(t *testing.T) {
	err := &ErrGoalAchieved{
		Condition:  "all tests pass",
		Iterations: 3,
		DurationMs: 9999,
		Tokens:     500,
	}
	assert.Contains(t, err.Error(), "all tests pass")
	assert.Contains(t, err.Error(), "3 iteration")
}

// TestIntegrationGoal is a skeleton for end-to-end coverage of WithGoal. It
// is permanently skipped because /goal evaluation requires a trusted
// workspace with hooks enabled, which the standard CI sandbox does not
// provide. Kept in place so the surface remains documented and the skeleton
// is ready when a suitable harness is available.
func TestIntegrationGoal(t *testing.T) {
	t.Skip("goal requires trusted workspace + hooks enabled")
}
