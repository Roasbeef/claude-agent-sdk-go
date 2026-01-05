package claudeagent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAskUserQuestionInputParsing(t *testing.T) {
	t.Run("parses single question with options", func(t *testing.T) {
		input := `{
			"questions": [{
				"question": "Which database should we use?",
				"header": "Database",
				"options": [
					{"label": "PostgreSQL", "description": "Relational database"},
					{"label": "MongoDB", "description": "Document database"}
				],
				"multiSelect": false
			}]
		}`

		var parsed AskUserQuestionInput
		err := json.Unmarshal([]byte(input), &parsed)
		require.NoError(t, err)

		assert.Len(t, parsed.Questions, 1)
		q := parsed.Questions[0]
		assert.Equal(t, "Which database should we use?", q.Question)
		assert.Equal(t, "Database", q.Header)
		assert.Len(t, q.Options, 2)
		assert.Equal(t, "PostgreSQL", q.Options[0].Label)
		assert.Equal(t, "Relational database", q.Options[0].Description)
		assert.False(t, q.MultiSelect)
	})

	t.Run("parses multiple questions", func(t *testing.T) {
		input := `{
			"questions": [
				{"question": "First question?", "header": "Q1", "options": []},
				{"question": "Second question?", "header": "Q2", "multiSelect": true}
			]
		}`

		var parsed AskUserQuestionInput
		err := json.Unmarshal([]byte(input), &parsed)
		require.NoError(t, err)

		assert.Len(t, parsed.Questions, 2)
		assert.Equal(t, "First question?", parsed.Questions[0].Question)
		assert.Equal(t, "Second question?", parsed.Questions[1].Question)
		assert.True(t, parsed.Questions[1].MultiSelect)
	})

	t.Run("handles missing optional fields", func(t *testing.T) {
		input := `{
			"questions": [{
				"question": "Simple question?"
			}]
		}`

		var parsed AskUserQuestionInput
		err := json.Unmarshal([]byte(input), &parsed)
		require.NoError(t, err)

		assert.Len(t, parsed.Questions, 1)
		assert.Equal(t, "Simple question?", parsed.Questions[0].Question)
		assert.Empty(t, parsed.Questions[0].Header)
		assert.Empty(t, parsed.Questions[0].Options)
		assert.False(t, parsed.Questions[0].MultiSelect)
	})
}

func TestQuestionSetAnswer(t *testing.T) {
	qs := QuestionSet{
		ToolUseID: "tool_123",
		Questions: []QuestionItem{
			{Question: "Q1?", Options: []QuestionOption{{Label: "Yes"}, {Label: "No"}}},
			{Question: "Q2?", Options: []QuestionOption{{Label: "A"}, {Label: "B"}}},
		},
		SessionID: "session_456",
	}

	t.Run("Answer creates single-question answer map", func(t *testing.T) {
		answers := qs.Answer(0, "Yes")
		assert.Equal(t, Answers{"q_0": "Yes"}, answers)

		answers = qs.Answer(1, "B")
		assert.Equal(t, Answers{"q_1": "B"}, answers)
	})

	t.Run("AnswerAll combines multiple answers", func(t *testing.T) {
		answers := qs.AnswerAll(
			QuestionAnswer{Key: "q_0", Value: "Yes"},
			QuestionAnswer{Key: "q_1", Value: "A"},
		)
		assert.Equal(t, Answers{"q_0": "Yes", "q_1": "A"}, answers)
	})
}

func TestQuestionRef(t *testing.T) {
	qs := QuestionSet{
		ToolUseID: "tool_123",
		Questions: []QuestionItem{
			{
				Question: "Choose a database",
				Options: []QuestionOption{
					{Label: "PostgreSQL", Description: "SQL"},
					{Label: "MongoDB", Description: "NoSQL"},
					{Label: "Redis", Description: "Cache"},
				},
			},
			{
				Question: "Freeform question",
			},
		},
	}

	t.Run("Q returns nil for out of range", func(t *testing.T) {
		assert.Nil(t, qs.Q(-1))
		assert.Nil(t, qs.Q(10))
	})

	t.Run("Q returns valid ref for valid index", func(t *testing.T) {
		ref := qs.Q(0)
		assert.NotNil(t, ref)
	})

	t.Run("Select creates answer with label", func(t *testing.T) {
		ref := qs.Q(0)
		answer := ref.Select("MongoDB")
		assert.Equal(t, "q_0", answer.Key)
		assert.Equal(t, "MongoDB", answer.Value)
	})

	t.Run("SelectIndex creates answer from option index", func(t *testing.T) {
		ref := qs.Q(0)

		answer := ref.SelectIndex(0)
		assert.Equal(t, "q_0", answer.Key)
		assert.Equal(t, "PostgreSQL", answer.Value)

		answer = ref.SelectIndex(2)
		assert.Equal(t, "Redis", answer.Value)
	})

	t.Run("SelectIndex handles out of range option", func(t *testing.T) {
		ref := qs.Q(0)
		answer := ref.SelectIndex(99)
		assert.Equal(t, "q_0", answer.Key)
		assert.Equal(t, "", answer.Value)
	})

	t.Run("SelectIndex handles negative option index", func(t *testing.T) {
		ref := qs.Q(0)
		answer := ref.SelectIndex(-1)
		assert.Equal(t, "q_0", answer.Key)
		assert.Equal(t, "", answer.Value)
	})

	t.Run("SelectIndex handles nil QuestionSet", func(t *testing.T) {
		// Manually create a QuestionRef with nil qs (shouldn't happen in normal use)
		ref := &QuestionRef{qs: nil, index: 0}
		answer := ref.SelectIndex(0)
		assert.Equal(t, "q_0", answer.Key)
		assert.Equal(t, "", answer.Value)
	})

	t.Run("SelectIndex handles invalid question index", func(t *testing.T) {
		// Create ref with out-of-range question index
		ref := &QuestionRef{qs: &qs, index: 99}
		answer := ref.SelectIndex(0)
		assert.Equal(t, "q_99", answer.Key)
		assert.Equal(t, "", answer.Value)
	})

	t.Run("SelectMultiple joins labels", func(t *testing.T) {
		ref := qs.Q(0)
		answer := ref.SelectMultiple("PostgreSQL", "Redis")
		assert.Equal(t, "q_0", answer.Key)
		assert.Equal(t, "PostgreSQL, Redis", answer.Value)
	})

	t.Run("Text creates freeform answer", func(t *testing.T) {
		ref := qs.Q(1)
		answer := ref.Text("Custom response here")
		assert.Equal(t, "q_1", answer.Key)
		assert.Equal(t, "Custom response here", answer.Value)
	})
}

func TestQuestionSetAnswerAllWithRefs(t *testing.T) {
	qs := QuestionSet{
		ToolUseID: "tool_123",
		Questions: []QuestionItem{
			{Question: "DB?", Options: []QuestionOption{{Label: "Postgres"}, {Label: "MySQL"}}},
			{Question: "Cache?", Options: []QuestionOption{{Label: "Redis"}, {Label: "Memcached"}}},
			{Question: "Notes?"},
		},
	}

	answers := qs.AnswerAll(
		qs.Q(0).Select("Postgres"),
		qs.Q(1).SelectIndex(0),
		qs.Q(2).Text("Using defaults"),
	)

	expected := Answers{
		"q_0": "Postgres",
		"q_1": "Redis",
		"q_2": "Using defaults",
	}

	assert.Equal(t, expected, answers)
}

func TestAnswersJSONSerialization(t *testing.T) {
	answers := Answers{
		"q_0": "Option A",
		"q_1": "Custom text",
	}

	data, err := json.Marshal(answers)
	require.NoError(t, err)

	var parsed Answers
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, answers, parsed)
}

func TestToolResultFormat(t *testing.T) {
	// Test that the answer format matches what Claude expects.
	answers := Answers{"q_0": "selected_option"}

	result := map[string]interface{}{
		"answers": answers,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	// Should produce: {"answers":{"q_0":"selected_option"}}
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	answersMap, ok := parsed["answers"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "selected_option", answersMap["q_0"])
}

func TestToolErrorResultFormat(t *testing.T) {
	// Test that error results are formatted correctly for Claude.
	errMsg := "question handler error: simulated failure"

	result := map[string]interface{}{
		"error": errMsg,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	// Should produce: {"error":"question handler error: simulated failure"}
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	errorStr, ok := parsed["error"].(string)
	require.True(t, ok)
	assert.Contains(t, errorStr, "question handler error")
	assert.Contains(t, errorStr, "simulated failure")
}

func TestQuestionMessage(t *testing.T) {
	t.Run("MessageType returns question", func(t *testing.T) {
		qm := QuestionMessage{}
		assert.Equal(t, "question", qm.MessageType())
	})

	t.Run("embeds QuestionSet methods", func(t *testing.T) {
		qm := QuestionMessage{
			QuestionSet: QuestionSet{
				ToolUseID: "tool_123",
				Questions: []QuestionItem{
					{
						Question: "Choose a database",
						Options: []QuestionOption{
							{Label: "PostgreSQL"},
							{Label: "MySQL"},
						},
					},
				},
			},
		}

		// Can use Answer() directly on QuestionMessage
		answers := qm.Answer(0, "PostgreSQL")
		assert.Equal(t, Answers{"q_0": "PostgreSQL"}, answers)

		// Can use Q() directly on QuestionMessage
		ref := qm.Q(0)
		assert.NotNil(t, ref)
		answer := ref.SelectIndex(1)
		assert.Equal(t, "MySQL", answer.Value)
	})

	t.Run("Respond calls responder function", func(t *testing.T) {
		var receivedAnswers Answers

		qm := QuestionMessage{
			QuestionSet: QuestionSet{
				ToolUseID: "tool_123",
				Questions: []QuestionItem{{Question: "Test?"}},
			},
			responder: func(answers Answers) error {
				receivedAnswers = answers
				return nil
			},
		}

		err := qm.Respond(Answers{"q_0": "test_answer"})
		require.NoError(t, err)
		assert.Equal(t, "test_answer", receivedAnswers["q_0"])
	})

	t.Run("Respond returns error if no responder", func(t *testing.T) {
		qm := QuestionMessage{
			QuestionSet: QuestionSet{
				ToolUseID: "tool_123",
			},
			responder: nil,
		}

		err := qm.Respond(Answers{"q_0": "test"})
		require.Error(t, err)

		var notFoundErr *ErrQuestionNotFound
		assert.ErrorAs(t, err, &notFoundErr)
		assert.Equal(t, "tool_123", notFoundErr.ToolUseID)
	})

	t.Run("Respond with fluent helpers", func(t *testing.T) {
		var receivedAnswers Answers

		qm := QuestionMessage{
			QuestionSet: QuestionSet{
				ToolUseID: "tool_456",
				Questions: []QuestionItem{
					{
						Question: "DB?",
						Options:  []QuestionOption{{Label: "Postgres"}, {Label: "MySQL"}},
					},
					{
						Question: "Notes?",
					},
				},
			},
			responder: func(answers Answers) error {
				receivedAnswers = answers
				return nil
			},
		}

		// Use fluent API directly on QuestionMessage
		err := qm.Respond(qm.AnswerAll(
			qm.Q(0).SelectIndex(0),
			qm.Q(1).Text("No notes"),
		))

		require.NoError(t, err)
		assert.Equal(t, "Postgres", receivedAnswers["q_0"])
		assert.Equal(t, "No notes", receivedAnswers["q_1"])
	})
}

func TestQuestionMessageAsMessage(t *testing.T) {
	// Verify QuestionMessage satisfies the Message interface
	var msg Message = QuestionMessage{
		QuestionSet: QuestionSet{ToolUseID: "test"},
	}

	assert.Equal(t, "question", msg.MessageType())
}
