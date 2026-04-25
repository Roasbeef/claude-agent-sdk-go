package claudeagent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSDKControlRequestBodyInitializeOptions(t *testing.T) {
	trueVal := true
	falseVal := false

	body := SDKControlRequestBody{
		Subtype:                "initialize",
		PlanModeInstructions:   "plan first",
		ExcludeDynamicSections: &trueVal,
		Title:                  "custom title",
		Skills:                 []string{"go", "review"},
		PromptSuggestions:      &falseVal,
		AgentProgressSummaries: &trueVal,
		ForwardSubagentText:    &falseVal,
	}

	data, err := json.Marshal(body)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "initialize", got["subtype"])
	assert.Equal(t, "plan first", got["planModeInstructions"])
	assert.Equal(t, true, got["excludeDynamicSections"])
	assert.Equal(t, "custom title", got["title"])
	assert.Equal(t, []interface{}{"go", "review"}, got["skills"])
	assert.Equal(t, false, got["promptSuggestions"])
	assert.Equal(t, true, got["agentProgressSummaries"])
	assert.Equal(t, false, got["forwardSubagentText"])
}

func TestSDKControlRequestBodyInitializeOptionsOmitUnset(t *testing.T) {
	data, err := json.Marshal(SDKControlRequestBody{Subtype: "initialize"})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))

	for _, key := range []string{
		"planModeInstructions",
		"excludeDynamicSections",
		"title",
		"skills",
		"promptSuggestions",
		"agentProgressSummaries",
		"forwardSubagentText",
	} {
		assert.NotContains(t, got, key)
	}
}

func TestAgentDefinitionJSON(t *testing.T) {
	background := false
	effortBudget := 30000
	agent := AgentDefinition{
		Name:                               "reviewer",
		Description:                        "Reviews Go changes",
		Prompt:                             "Review carefully",
		Tools:                              []string{"Read", "Grep"},
		Model:                              "claude-opus-4-5-20250929",
		DisallowedTools:                    []string{"Bash"},
		MCPServers:                         []AgentMCPServerSpec{{Name: "github"}},
		CriticalSystemReminderExperimental: "Stay focused",
		Skills:                             []string{"go"},
		InitialPrompt:                      "Start here",
		MaxTurns:                           5,
		Background:                         &background,
		Memory:                             AgentMemoryProject,
		Effort:                             AgentEffort{Numeric: &effortBudget},
		PermissionMode:                     PermissionModeAcceptEdits,
	}

	data, err := json.Marshal(agent)
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))

	assert.NotContains(t, got, "Name")
	assert.NotContains(t, got, "name")
	assert.Equal(t, "Reviews Go changes", got["description"])
	assert.Equal(t, "Review carefully", got["prompt"])
	assert.Equal(t, []interface{}{"Read", "Grep"}, got["tools"])
	assert.Equal(t, "claude-opus-4-5-20250929", got["model"])
	assert.Equal(t, []interface{}{"Bash"}, got["disallowedTools"])
	assert.Equal(t, []interface{}{"github"}, got["mcpServers"])
	assert.Equal(t, "Stay focused", got["criticalSystemReminder_EXPERIMENTAL"])
	assert.Equal(t, []interface{}{"go"}, got["skills"])
	assert.Equal(t, "Start here", got["initialPrompt"])
	assert.Equal(t, float64(5), got["maxTurns"])
	assert.Equal(t, false, got["background"])
	assert.Equal(t, "project", got["memory"])
	assert.Equal(t, float64(30000), got["effort"])
	assert.Equal(t, "acceptEdits", got["permissionMode"])

	var decoded AgentDefinition
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, agent.Description, decoded.Description)
	assert.Equal(t, agent.Prompt, decoded.Prompt)
	assert.Equal(t, agent.Tools, decoded.Tools)
	assert.Equal(t, agent.Model, decoded.Model)
	assert.Equal(t, agent.DisallowedTools, decoded.DisallowedTools)
	require.Len(t, decoded.MCPServers, 1)
	assert.Equal(t, "github", decoded.MCPServers[0].Name)
	assert.Equal(t, agent.CriticalSystemReminderExperimental, decoded.CriticalSystemReminderExperimental)
	assert.Equal(t, agent.Skills, decoded.Skills)
	assert.Equal(t, agent.InitialPrompt, decoded.InitialPrompt)
	assert.Equal(t, agent.MaxTurns, decoded.MaxTurns)
	require.NotNil(t, decoded.Background)
	assert.Equal(t, false, *decoded.Background)
	assert.Equal(t, agent.Memory, decoded.Memory)
	require.NotNil(t, decoded.Effort.Numeric)
	assert.Equal(t, effortBudget, *decoded.Effort.Numeric)
	assert.Equal(t, agent.PermissionMode, decoded.PermissionMode)
}

func TestAgentDefinitionJSONOmitUnset(t *testing.T) {
	data, err := json.Marshal(AgentDefinition{
		Description: "Reviews changes",
		Prompt:      "Review carefully",
	})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, "Reviews changes", got["description"])
	assert.Equal(t, "Review carefully", got["prompt"])
	for _, key := range []string{"memory", "effort", "maxTurns", "background"} {
		assert.NotContains(t, got, key)
	}
}

func TestAgentMCPServerSpecJSON(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		data, err := json.Marshal(AgentMCPServerSpec{Name: "github"})
		require.NoError(t, err)
		assert.JSONEq(t, `"github"`, string(data))

		var decoded AgentMCPServerSpec
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, "github", decoded.Name)
		assert.Nil(t, decoded.Inline)
	})

	t.Run("inline", func(t *testing.T) {
		spec := AgentMCPServerSpec{
			Inline: map[string]MCPServerConfig{
				"github": {
					Type:    "stdio",
					Command: "gh",
				},
			},
		}
		data, err := json.Marshal(spec)
		require.NoError(t, err)
		assert.JSONEq(t, `{"github":{"type":"stdio","command":"gh"}}`, string(data))

		var decoded AgentMCPServerSpec
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Contains(t, decoded.Inline, "github")
		assert.Equal(t, "stdio", decoded.Inline["github"].Type)
		assert.Equal(t, "gh", decoded.Inline["github"].Command)
		assert.Empty(t, decoded.Name)
	})

	t.Run("leading whitespace", func(t *testing.T) {
		var named AgentMCPServerSpec
		require.NoError(t, json.Unmarshal([]byte("  \n\t\"github\""), &named))
		assert.Equal(t, "github", named.Name)

		var inline AgentMCPServerSpec
		require.NoError(t, json.Unmarshal([]byte("\n {\"github\":{\"type\":\"stdio\",\"command\":\"gh\"}}"), &inline))
		require.Contains(t, inline.Inline, "github")
		assert.Equal(t, "stdio", inline.Inline["github"].Type)
	})
}

func TestAgentEffortJSON(t *testing.T) {
	t.Run("level", func(t *testing.T) {
		data, err := json.Marshal(AgentEffort{Level: EffortHigh})
		require.NoError(t, err)
		assert.JSONEq(t, `"high"`, string(data))

		var decoded AgentEffort
		require.NoError(t, json.Unmarshal(data, &decoded))
		assert.Equal(t, EffortHigh, decoded.Level)
		assert.Nil(t, decoded.Numeric)
	})

	t.Run("numeric", func(t *testing.T) {
		budget := 30000
		data, err := json.Marshal(AgentEffort{Numeric: &budget})
		require.NoError(t, err)
		assert.JSONEq(t, `30000`, string(data))

		var decoded AgentEffort
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.NotNil(t, decoded.Numeric)
		assert.Equal(t, budget, *decoded.Numeric)
		assert.Empty(t, decoded.Level)
	})
}

func TestAgentMemoryScopeJSON(t *testing.T) {
	for _, tt := range []struct {
		name  string
		scope AgentMemoryScope
		want  string
	}{
		{"user", AgentMemoryUser, `"user"`},
		{"project", AgentMemoryProject, `"project"`},
		{"local", AgentMemoryLocal, `"local"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.scope)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(data))
		})
	}
}

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

func TestParseMessageSystemInit(t *testing.T) {
	input := `{
		"type": "system",
		"subtype": "init",
		"uuid": "550e8400-e29b-41d4-a716-446655440000",
		"session_id": "sess_hook_123",
		"apiKeySource": "env",
		"cwd": "/workspace/project",
		"tools": ["Read", "Edit"],
		"mcp_servers": [{"name": "github", "status": "connected"}],
		"model": "claude-opus-4-5-20250929",
		"permissionMode": "acceptEdits",
		"slash_commands": ["/help"],
		"output_style": "default"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	systemMsg, ok := msg.(SystemMessage)
	require.True(t, ok, "expected SystemMessage")

	assert.Equal(t, "system", systemMsg.MessageType())
	assert.Equal(t, "init", systemMsg.Subtype)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", systemMsg.UUID)
	assert.Equal(t, "sess_hook_123", systemMsg.SessionID)
	assert.Equal(t, "env", systemMsg.APIKeySource)
	assert.Equal(t, "/workspace/project", systemMsg.Cwd)
	assert.Equal(t, []string{"Read", "Edit"}, systemMsg.Tools)
	require.Len(t, systemMsg.MCPServers, 1)
	assert.Equal(t, "github", systemMsg.MCPServers[0].Name)
	assert.Equal(t, "connected", systemMsg.MCPServers[0].Status)
	assert.Equal(t, "claude-opus-4-5-20250929", systemMsg.Model)
	assert.Equal(t, PermissionModeAcceptEdits, systemMsg.PermissionMode)
	assert.Equal(t, []string{"/help"}, systemMsg.SlashCommands)
	assert.Equal(t, "default", systemMsg.OutputStyle)
}

func TestParseMessageCompactBoundary(t *testing.T) {
	input := `{
		"type": "system",
		"subtype": "compact_boundary",
		"uuid": "550e8400-e29b-41d4-a716-446655440010",
		"session_id": "sess_compact_123",
		"compact_metadata": {
			"trigger": "auto",
			"pre_tokens": 198732
		}
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	compactMsg, ok := msg.(CompactBoundaryMessage)
	require.True(t, ok, "expected CompactBoundaryMessage")

	assert.Equal(t, "system", compactMsg.MessageType())
	assert.Equal(t, "system", compactMsg.Type)
	assert.Equal(t, "compact_boundary", compactMsg.Subtype)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440010", compactMsg.UUID)
	assert.Equal(t, "sess_compact_123", compactMsg.SessionID)
	assert.Equal(t, "auto", compactMsg.CompactMetadata.Trigger)
	assert.Equal(t, 198732, compactMsg.CompactMetadata.PreTokens)
}

func TestParseMessageHookStarted(t *testing.T) {
	input := `{
		"type": "system",
		"subtype": "hook_started",
		"hook_id": "hook_01J8Z8Y2X3K4M5N6P7Q8R9S0T1",
		"hook_name": "format-go",
		"hook_event": "PostToolUse",
		"uuid": "550e8400-e29b-41d4-a716-446655440001",
		"session_id": "sess_hook_123"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	hookMsg, ok := msg.(HookStartedMessage)
	require.True(t, ok, "expected HookStartedMessage")

	assert.Equal(t, "system", hookMsg.MessageType())
	assert.Equal(t, "system", hookMsg.Type)
	assert.Equal(t, "hook_started", hookMsg.Subtype)
	assert.Equal(t, "hook_01J8Z8Y2X3K4M5N6P7Q8R9S0T1", hookMsg.HookID)
	assert.Equal(t, "format-go", hookMsg.HookName)
	assert.Equal(t, "PostToolUse", hookMsg.HookEvent)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", hookMsg.UUID)
	assert.Equal(t, "sess_hook_123", hookMsg.SessionID)
}

func TestParseMessageHookProgress(t *testing.T) {
	input := `{
		"type": "system",
		"subtype": "hook_progress",
		"hook_id": "hook_01J8Z8Y2X3K4M5N6P7Q8R9S0T2",
		"hook_name": "format-go",
		"hook_event": "PostToolUse",
		"stdout": "gofmt messages.go\n",
		"stderr": "",
		"output": "gofmt messages.go\n",
		"uuid": "550e8400-e29b-41d4-a716-446655440002",
		"session_id": "sess_hook_123"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	hookMsg, ok := msg.(HookProgressMessage)
	require.True(t, ok, "expected HookProgressMessage")

	assert.Equal(t, "system", hookMsg.MessageType())
	assert.Equal(t, "system", hookMsg.Type)
	assert.Equal(t, "hook_progress", hookMsg.Subtype)
	assert.Equal(t, "hook_01J8Z8Y2X3K4M5N6P7Q8R9S0T2", hookMsg.HookID)
	assert.Equal(t, "format-go", hookMsg.HookName)
	assert.Equal(t, "PostToolUse", hookMsg.HookEvent)
	assert.Equal(t, "gofmt messages.go\n", hookMsg.Stdout)
	assert.Empty(t, hookMsg.Stderr)
	assert.Equal(t, "gofmt messages.go\n", hookMsg.Output)
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440002", hookMsg.UUID)
	assert.Equal(t, "sess_hook_123", hookMsg.SessionID)
}

func TestParseMessageHookResponse(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantOutcome  HookOutcome
		wantExitCode *int
	}{
		{
			name: "success with exit code",
			input: `{
				"type": "system",
				"subtype": "hook_response",
				"hook_id": "hook_01J8Z8Y2X3K4M5N6P7Q8R9S0T3",
				"hook_name": "format-go",
				"hook_event": "PostToolUse",
				"output": "formatted files\n",
				"stdout": "formatted files\n",
				"stderr": "",
				"exit_code": 0,
				"outcome": "success",
				"uuid": "550e8400-e29b-41d4-a716-446655440003",
				"session_id": "sess_hook_123"
			}`,
			wantOutcome:  HookOutcomeSuccess,
			wantExitCode: intPtr(0),
		},
		{
			name: "error without exit code",
			input: `{
				"type": "system",
				"subtype": "hook_response",
				"hook_id": "hook_01J8Z8Y2X3K4M5N6P7Q8R9S0T4",
				"hook_name": "lint-go",
				"hook_event": "PostToolUse",
				"output": "lint failed\n",
				"stdout": "",
				"stderr": "lint failed\n",
				"outcome": "error",
				"uuid": "550e8400-e29b-41d4-a716-446655440004",
				"session_id": "sess_hook_123"
			}`,
			wantOutcome: HookOutcomeError,
		},
		{
			name: "canceled without exit code",
			input: `{
				"type": "system",
				"subtype": "hook_response",
				"hook_id": "hook_01J8Z8Y2X3K4M5N6P7Q8R9S0T5",
				"hook_name": "slow-check",
				"hook_event": "PreToolUse",
				"output": "canceled by user\n",
				"stdout": "",
				"stderr": "",
				"outcome": "cancelled",
				"uuid": "550e8400-e29b-41d4-a716-446655440005",
				"session_id": "sess_hook_123"
			}`, //nolint:misspell // upstream wire format spelling
			wantOutcome: HookOutcomeCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			hookMsg, ok := msg.(HookResponseMessage)
			require.True(t, ok, "expected HookResponseMessage")

			assert.Equal(t, "system", hookMsg.MessageType())
			assert.Equal(t, "system", hookMsg.Type)
			assert.Equal(t, "hook_response", hookMsg.Subtype)
			assert.NotEmpty(t, hookMsg.HookID)
			assert.NotEmpty(t, hookMsg.HookName)
			assert.NotEmpty(t, hookMsg.HookEvent)
			assert.NotEmpty(t, hookMsg.Output)
			assert.Equal(t, tt.wantOutcome, hookMsg.Outcome)
			if tt.wantExitCode == nil {
				assert.Nil(t, hookMsg.ExitCode)
			} else {
				require.NotNil(t, hookMsg.ExitCode)
				assert.Equal(t, *tt.wantExitCode, *hookMsg.ExitCode)
			}
			assert.NotEmpty(t, hookMsg.UUID)
			assert.Equal(t, "sess_hook_123", hookMsg.SessionID)
		})
	}
}

func TestParseMessageTaskStarted(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, msg TaskStartedMessage)
	}{
		{
			name: "all fields populated",
			input: `{
				"type": "system",
				"subtype": "task_started",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0T6",
				"tool_use_id": "toolu_01J8Z8Y2X3K4M5N6P7Q8R9S0T7",
				"description": "Run repository checks",
				"task_type": "local_workflow",
				"workflow_name": "ci-checks",
				"prompt": "Run the Go validation workflow",
				"skip_transcript": false,
				"uuid": "550e8400-e29b-41d4-a716-446655440011",
				"session_id": "sess_task_123"
			}`,
			check: func(t *testing.T, taskMsg TaskStartedMessage) {
				t.Helper()
				assert.Equal(t, "task_01J8Z8Y2X3K4M5N6P7Q8R9S0T6", taskMsg.TaskID)
				assert.Equal(t, "toolu_01J8Z8Y2X3K4M5N6P7Q8R9S0T7", taskMsg.ToolUseID)
				assert.Equal(t, "Run repository checks", taskMsg.Description)
				assert.Equal(t, "local_workflow", taskMsg.TaskType)
				assert.Equal(t, "ci-checks", taskMsg.WorkflowName)
				assert.Equal(t, "Run the Go validation workflow", taskMsg.Prompt)
				require.NotNil(t, taskMsg.SkipTranscript)
				assert.False(t, *taskMsg.SkipTranscript)
			},
		},
		{
			name: "minimum required fields",
			input: `{
				"type": "system",
				"subtype": "task_started",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0T8",
				"description": "Summarize repository status",
				"uuid": "550e8400-e29b-41d4-a716-446655440012",
				"session_id": "sess_task_123"
			}`,
			check: func(t *testing.T, taskMsg TaskStartedMessage) {
				t.Helper()
				assert.Equal(t, "task_01J8Z8Y2X3K4M5N6P7Q8R9S0T8", taskMsg.TaskID)
				assert.Equal(t, "Summarize repository status", taskMsg.Description)
				assert.Empty(t, taskMsg.ToolUseID)
				assert.Empty(t, taskMsg.TaskType)
				assert.Empty(t, taskMsg.WorkflowName)
				assert.Empty(t, taskMsg.Prompt)
				assert.Nil(t, taskMsg.SkipTranscript)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			taskMsg, ok := msg.(TaskStartedMessage)
			require.True(t, ok, "expected TaskStartedMessage")

			assert.Equal(t, "system", taskMsg.MessageType())
			assert.Equal(t, "system", taskMsg.Type)
			assert.Equal(t, "task_started", taskMsg.Subtype)
			assert.NotEmpty(t, taskMsg.UUID)
			assert.Equal(t, "sess_task_123", taskMsg.SessionID)
			tt.check(t, taskMsg)
		})
	}
}

func TestParseMessageTaskProgress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, msg TaskProgressMessage)
	}{
		{
			name: "with optional progress fields",
			input: `{
				"type": "system",
				"subtype": "task_progress",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0T9",
				"tool_use_id": "toolu_01J8Z8Y2X3K4M5N6P7Q8R9S0TA",
				"description": "Run repository checks",
				"usage": {
					"total_tokens": 1200,
					"tool_uses": 3,
					"duration_ms": 4500
				},
				"last_tool_name": "Bash",
				"summary": "Tests are still running",
				"uuid": "550e8400-e29b-41d4-a716-446655440013",
				"session_id": "sess_task_123"
			}`,
			check: func(t *testing.T, taskMsg TaskProgressMessage) {
				t.Helper()
				assert.Equal(t, "toolu_01J8Z8Y2X3K4M5N6P7Q8R9S0TA", taskMsg.ToolUseID)
				assert.Equal(t, "Bash", taskMsg.LastToolName)
				assert.Equal(t, "Tests are still running", taskMsg.Summary)
			},
		},
		{
			name: "required usage only",
			input: `{
				"type": "system",
				"subtype": "task_progress",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0TB",
				"description": "Collect task output",
				"usage": {
					"total_tokens": 12,
					"tool_uses": 1,
					"duration_ms": 80
				},
				"uuid": "550e8400-e29b-41d4-a716-446655440014",
				"session_id": "sess_task_123"
			}`,
			check: func(t *testing.T, taskMsg TaskProgressMessage) {
				t.Helper()
				assert.Empty(t, taskMsg.ToolUseID)
				assert.Empty(t, taskMsg.LastToolName)
				assert.Empty(t, taskMsg.Summary)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			taskMsg, ok := msg.(TaskProgressMessage)
			require.True(t, ok, "expected TaskProgressMessage")

			assert.Equal(t, "system", taskMsg.MessageType())
			assert.Equal(t, "system", taskMsg.Type)
			assert.Equal(t, "task_progress", taskMsg.Subtype)
			assert.NotEmpty(t, taskMsg.TaskID)
			assert.NotEmpty(t, taskMsg.Description)
			assert.NotEmpty(t, taskMsg.UUID)
			assert.Equal(t, "sess_task_123", taskMsg.SessionID)
			assert.NotZero(t, taskMsg.Usage.TotalTokens)
			assert.NotZero(t, taskMsg.Usage.ToolUses)
			assert.NotZero(t, taskMsg.Usage.DurationMS)
			tt.check(t, taskMsg)
		})
	}
}

func TestParseMessageTaskUpdated(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, msg TaskUpdatedMessage)
	}{
		{
			name: "status only",
			input: `{
				"type": "system",
				"subtype": "task_updated",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0TC",
				"patch": {
					"status": "running"
				},
				"uuid": "550e8400-e29b-41d4-a716-446655440015",
				"session_id": "sess_task_123"
			}`,
			check: func(t *testing.T, taskMsg TaskUpdatedMessage) {
				t.Helper()
				assert.Equal(t, TaskRunStatusRunning, taskMsg.Patch.Status)
				assert.Empty(t, taskMsg.Patch.Error)
				assert.Nil(t, taskMsg.Patch.EndTime)
				assert.Nil(t, taskMsg.Patch.TotalPausedMS)
				assert.Nil(t, taskMsg.Patch.IsBackgrounded)
			},
		},
		{
			name: "error only",
			input: `{
				"type": "system",
				"subtype": "task_updated",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0TD",
				"patch": {
					"error": "workflow exited with status 1"
				},
				"uuid": "550e8400-e29b-41d4-a716-446655440016",
				"session_id": "sess_task_123"
			}`,
			check: func(t *testing.T, taskMsg TaskUpdatedMessage) {
				t.Helper()
				assert.Empty(t, taskMsg.Patch.Status)
				assert.Equal(t, "workflow exited with status 1", taskMsg.Patch.Error)
				assert.Nil(t, taskMsg.Patch.EndTime)
				assert.Nil(t, taskMsg.Patch.TotalPausedMS)
				assert.Nil(t, taskMsg.Patch.IsBackgrounded)
			},
		},
		{
			name: "populated patch",
			input: `{
				"type": "system",
				"subtype": "task_updated",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0TE",
				"patch": {
					"status": "completed",
					"description": "Repository checks completed",
					"end_time": 1763856000123,
					"total_paused_ms": 250,
					"is_backgrounded": true
				},
				"uuid": "550e8400-e29b-41d4-a716-446655440017",
				"session_id": "sess_task_123"
			}`,
			check: func(t *testing.T, taskMsg TaskUpdatedMessage) {
				t.Helper()
				assert.Equal(t, TaskRunStatusCompleted, taskMsg.Patch.Status)
				assert.Equal(t, "Repository checks completed", taskMsg.Patch.Description)
				require.NotNil(t, taskMsg.Patch.EndTime)
				assert.Equal(t, int64(1763856000123), *taskMsg.Patch.EndTime)
				require.NotNil(t, taskMsg.Patch.TotalPausedMS)
				assert.Equal(t, int64(250), *taskMsg.Patch.TotalPausedMS)
				require.NotNil(t, taskMsg.Patch.IsBackgrounded)
				assert.True(t, *taskMsg.Patch.IsBackgrounded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			taskMsg, ok := msg.(TaskUpdatedMessage)
			require.True(t, ok, "expected TaskUpdatedMessage")

			assert.Equal(t, "system", taskMsg.MessageType())
			assert.Equal(t, "system", taskMsg.Type)
			assert.Equal(t, "task_updated", taskMsg.Subtype)
			assert.NotEmpty(t, taskMsg.TaskID)
			assert.NotEmpty(t, taskMsg.UUID)
			assert.Equal(t, "sess_task_123", taskMsg.SessionID)
			tt.check(t, taskMsg)
		})
	}
}

func TestParseMessageTaskNotification(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantStatus TaskNotificationStatus
		check      func(t *testing.T, msg TaskNotificationMessage)
	}{
		{
			name: "completed with usage",
			input: `{
				"type": "system",
				"subtype": "task_notification",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0TF",
				"tool_use_id": "toolu_01J8Z8Y2X3K4M5N6P7Q8R9S0TG",
				"status": "completed",
				"output_file": "/tmp/claude-task-output.md",
				"summary": "Checks completed successfully",
				"usage": {
					"total_tokens": 2400,
					"tool_uses": 4,
					"duration_ms": 9100
				},
				"skip_transcript": true,
				"uuid": "550e8400-e29b-41d4-a716-446655440018",
				"session_id": "sess_task_123"
			}`,
			wantStatus: TaskNotificationStatusCompleted,
			check: func(t *testing.T, taskMsg TaskNotificationMessage) {
				t.Helper()
				assert.Equal(t, "toolu_01J8Z8Y2X3K4M5N6P7Q8R9S0TG", taskMsg.ToolUseID)
				require.NotNil(t, taskMsg.Usage)
				assert.Equal(t, 2400, taskMsg.Usage.TotalTokens)
				assert.Equal(t, 4, taskMsg.Usage.ToolUses)
				assert.Equal(t, 9100, taskMsg.Usage.DurationMS)
				require.NotNil(t, taskMsg.SkipTranscript)
				assert.True(t, *taskMsg.SkipTranscript)
			},
		},
		{
			name: "failed without usage",
			input: `{
				"type": "system",
				"subtype": "task_notification",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0TH",
				"status": "failed",
				"output_file": "/tmp/claude-task-output-failed.md",
				"summary": "Checks failed",
				"uuid": "550e8400-e29b-41d4-a716-446655440019",
				"session_id": "sess_task_123"
			}`,
			wantStatus: TaskNotificationStatusFailed,
			check: func(t *testing.T, taskMsg TaskNotificationMessage) {
				t.Helper()
				assert.Empty(t, taskMsg.ToolUseID)
				assert.Nil(t, taskMsg.Usage)
				assert.Nil(t, taskMsg.SkipTranscript)
			},
		},
		{
			name: "stopped without usage",
			input: `{
				"type": "system",
				"subtype": "task_notification",
				"task_id": "task_01J8Z8Y2X3K4M5N6P7Q8R9S0TI",
				"status": "stopped",
				"output_file": "/tmp/claude-task-output-stopped.md",
				"summary": "Task stopped by request",
				"uuid": "550e8400-e29b-41d4-a716-446655440020",
				"session_id": "sess_task_123"
			}`,
			wantStatus: TaskNotificationStatusStopped,
			check: func(t *testing.T, taskMsg TaskNotificationMessage) {
				t.Helper()
				assert.Empty(t, taskMsg.ToolUseID)
				assert.Nil(t, taskMsg.Usage)
				assert.Nil(t, taskMsg.SkipTranscript)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			taskMsg, ok := msg.(TaskNotificationMessage)
			require.True(t, ok, "expected TaskNotificationMessage")

			assert.Equal(t, "system", taskMsg.MessageType())
			assert.Equal(t, "system", taskMsg.Type)
			assert.Equal(t, "task_notification", taskMsg.Subtype)
			assert.NotEmpty(t, taskMsg.TaskID)
			assert.Equal(t, tt.wantStatus, taskMsg.Status)
			assert.NotEmpty(t, taskMsg.OutputFile)
			assert.NotEmpty(t, taskMsg.Summary)
			assert.NotEmpty(t, taskMsg.UUID)
			assert.Equal(t, "sess_task_123", taskMsg.SessionID)
			tt.check(t, taskMsg)
		})
	}
}

func TestParseMessageAPIRetry(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantErrorStatus *int
		wantError       APIRetryError
	}{
		{
			name: "rate limit with HTTP status",
			input: `{
				"type": "system",
				"subtype": "api_retry",
				"attempt": 2,
				"max_retries": 5,
				"retry_delay_ms": 1500,
				"error_status": 429,
				"error": "rate_limit",
				"uuid": "550e8400-e29b-41d4-a716-446655440200",
				"session_id": "sess_misc_001"
			}`,
			wantErrorStatus: intPtr(429),
			wantError:       APIRetryErrorRateLimit,
		},
		{
			name: "connection error with null status",
			input: `{
				"type": "system",
				"subtype": "api_retry",
				"attempt": 1,
				"max_retries": 3,
				"retry_delay_ms": 250,
				"error_status": null,
				"error": "server_error",
				"uuid": "550e8400-e29b-41d4-a716-446655440201",
				"session_id": "sess_misc_001"
			}`,
			wantError: APIRetryErrorServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			retryMsg, ok := msg.(APIRetryMessage)
			require.True(t, ok, "expected APIRetryMessage")

			assert.Equal(t, "system", retryMsg.MessageType())
			assert.Equal(t, "api_retry", retryMsg.Subtype)
			assert.Equal(t, tt.wantError, retryMsg.Error)

			if tt.wantErrorStatus == nil {
				assert.Nil(t, retryMsg.ErrorStatus)
			} else {
				require.NotNil(t, retryMsg.ErrorStatus)
				assert.Equal(t, *tt.wantErrorStatus, *retryMsg.ErrorStatus)
			}
		})
	}
}

func TestParseMessageElicitationComplete(t *testing.T) {
	input := `{
		"type": "system",
		"subtype": "elicitation_complete",
		"mcp_server_name": "github",
		"elicitation_id": "elic_01HXYZ",
		"uuid": "550e8400-e29b-41d4-a716-446655440210",
		"session_id": "sess_misc_002"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	elicMsg, ok := msg.(ElicitationCompleteMessage)
	require.True(t, ok, "expected ElicitationCompleteMessage")

	assert.Equal(t, "system", elicMsg.MessageType())
	assert.Equal(t, "elicitation_complete", elicMsg.Subtype)
	assert.Equal(t, "github", elicMsg.MCPServerName)
	assert.Equal(t, "elic_01HXYZ", elicMsg.ElicitationID)
	assert.Equal(t, "sess_misc_002", elicMsg.SessionID)
}

func TestParseMessageFilesPersisted(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFiles  int
		wantFailed int
	}{
		{
			name: "successes and failures",
			input: `{
				"type": "system",
				"subtype": "files_persisted",
				"files": [
					{ "filename": "a.txt", "file_id": "file_001" },
					{ "filename": "b.txt", "file_id": "file_002" }
				],
				"failed": [
					{ "filename": "c.txt", "error": "io error" }
				],
				"processed_at": "2026-04-25T18:30:00Z",
				"uuid": "550e8400-e29b-41d4-a716-446655440220",
				"session_id": "sess_misc_003"
			}`,
			wantFiles:  2,
			wantFailed: 1,
		},
		{
			name: "only successes",
			input: `{
				"type": "system",
				"subtype": "files_persisted",
				"files": [
					{ "filename": "only.txt", "file_id": "file_010" }
				],
				"failed": [],
				"processed_at": "2026-04-25T18:31:00Z",
				"uuid": "550e8400-e29b-41d4-a716-446655440221",
				"session_id": "sess_misc_003"
			}`,
			wantFiles:  1,
			wantFailed: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			fpMsg, ok := msg.(FilesPersistedEvent)
			require.True(t, ok, "expected FilesPersistedEvent")

			assert.Equal(t, "system", fpMsg.MessageType())
			assert.Equal(t, "files_persisted", fpMsg.Subtype)
			assert.Len(t, fpMsg.Files, tt.wantFiles)
			assert.Len(t, fpMsg.Failed, tt.wantFailed)
			assert.NotEmpty(t, fpMsg.ProcessedAt)
			if tt.wantFiles > 0 {
				assert.NotEmpty(t, fpMsg.Files[0].Filename)
				assert.NotEmpty(t, fpMsg.Files[0].FileID)
			}
			if tt.wantFailed > 0 {
				assert.NotEmpty(t, fpMsg.Failed[0].Filename)
				assert.NotEmpty(t, fpMsg.Failed[0].Error)
			}
		})
	}
}

func TestParseMessageLocalCommandOutput(t *testing.T) {
	input := `{
		"type": "system",
		"subtype": "local_command_output",
		"content": "Usage: 1234 tokens this turn",
		"uuid": "550e8400-e29b-41d4-a716-446655440230",
		"session_id": "sess_misc_004"
	}`

	msg, err := ParseMessage([]byte(input))
	require.NoError(t, err)

	cmdMsg, ok := msg.(LocalCommandOutputMessage)
	require.True(t, ok, "expected LocalCommandOutputMessage")

	assert.Equal(t, "system", cmdMsg.MessageType())
	assert.Equal(t, "local_command_output", cmdMsg.Subtype)
	assert.Equal(t, "Usage: 1234 tokens this turn", cmdMsg.Content)
	assert.Equal(t, "sess_misc_004", cmdMsg.SessionID)
}

func TestParseMessageMemoryRecall(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantMode         MemoryRecallMode
		wantEntries      int
		wantContentEntry int // index of entry expected to have non-empty content; -1 = none
	}{
		{
			name: "select mode without content",
			input: `{
				"type": "system",
				"subtype": "memory_recall",
				"mode": "select",
				"memories": [
					{ "path": "/memo/a.md", "scope": "personal" },
					{ "path": "/memo/b.md", "scope": "team" }
				],
				"uuid": "550e8400-e29b-41d4-a716-446655440240",
				"session_id": "sess_misc_005"
			}`,
			wantMode:         MemoryRecallModeSelect,
			wantEntries:      2,
			wantContentEntry: -1,
		},
		{
			name: "synthesize mode with content",
			input: `{
				"type": "system",
				"subtype": "memory_recall",
				"mode": "synthesize",
				"memories": [
					{ "path": "<synthesis:/memo>", "scope": "team", "content": "Distilled paragraph." }
				],
				"uuid": "550e8400-e29b-41d4-a716-446655440241",
				"session_id": "sess_misc_005"
			}`,
			wantMode:         MemoryRecallModeSynthesize,
			wantEntries:      1,
			wantContentEntry: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			recallMsg, ok := msg.(MemoryRecallMessage)
			require.True(t, ok, "expected MemoryRecallMessage")

			assert.Equal(t, "system", recallMsg.MessageType())
			assert.Equal(t, "memory_recall", recallMsg.Subtype)
			assert.Equal(t, tt.wantMode, recallMsg.Mode)
			assert.Len(t, recallMsg.Memories, tt.wantEntries)

			for i, entry := range recallMsg.Memories {
				assert.NotEmpty(t, entry.Path)
				assert.NotEmpty(t, string(entry.Scope))
				if i == tt.wantContentEntry {
					assert.NotEmpty(t, entry.Content)
				} else {
					assert.Empty(t, entry.Content)
				}
			}
		})
	}
}

func TestParseMessageMirrorError(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSubpath string
	}{
		{
			name: "with subpath",
			input: `{
				"type": "system",
				"subtype": "mirror_error",
				"error": "store unavailable",
				"key": {
					"projectKey": "proj_001",
					"sessionId": "sess_xyz",
					"subpath": "transcript/v1"
				},
				"uuid": "550e8400-e29b-41d4-a716-446655440250",
				"session_id": "sess_misc_006"
			}`,
			wantSubpath: "transcript/v1",
		},
		{
			name: "without subpath",
			input: `{
				"type": "system",
				"subtype": "mirror_error",
				"error": "timeout after 3 retries",
				"key": {
					"projectKey": "proj_002",
					"sessionId": "sess_uvw"
				},
				"uuid": "550e8400-e29b-41d4-a716-446655440251",
				"session_id": "sess_misc_006"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			mirMsg, ok := msg.(MirrorErrorMessage)
			require.True(t, ok, "expected MirrorErrorMessage")

			assert.Equal(t, "system", mirMsg.MessageType())
			assert.Equal(t, "mirror_error", mirMsg.Subtype)
			assert.NotEmpty(t, mirMsg.Error)
			assert.NotEmpty(t, mirMsg.Key.ProjectKey)
			assert.NotEmpty(t, mirMsg.Key.SessionID)
			assert.Equal(t, tt.wantSubpath, mirMsg.Key.Subpath)
		})
	}
}

func TestParseMessageNotification(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantPriority  NotificationPriority
		wantColor     string
		wantTimeoutMS *int
	}{
		{
			name: "low with no optional fields",
			input: `{
				"type": "system",
				"subtype": "notification",
				"key": "low_test",
				"text": "low priority message",
				"priority": "low",
				"uuid": "550e8400-e29b-41d4-a716-446655440260",
				"session_id": "sess_misc_007"
			}`,
			wantPriority: NotificationPriorityLow,
		},
		{
			name: "medium with color and timeout",
			input: `{
				"type": "system",
				"subtype": "notification",
				"key": "medium_test",
				"text": "medium priority message",
				"priority": "medium",
				"color": "#ffaa00",
				"timeout_ms": 5000,
				"uuid": "550e8400-e29b-41d4-a716-446655440261",
				"session_id": "sess_misc_007"
			}`,
			wantPriority:  NotificationPriorityMedium,
			wantColor:     "#ffaa00",
			wantTimeoutMS: intPtr(5000),
		},
		{
			name: "high",
			input: `{
				"type": "system",
				"subtype": "notification",
				"key": "high_test",
				"text": "high priority",
				"priority": "high",
				"uuid": "550e8400-e29b-41d4-a716-446655440262",
				"session_id": "sess_misc_007"
			}`,
			wantPriority: NotificationPriorityHigh,
		},
		{
			name: "immediate",
			input: `{
				"type": "system",
				"subtype": "notification",
				"key": "immediate_test",
				"text": "act now",
				"priority": "immediate",
				"uuid": "550e8400-e29b-41d4-a716-446655440263",
				"session_id": "sess_misc_007"
			}`,
			wantPriority: NotificationPriorityImmediate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			notifMsg, ok := msg.(NotificationMessage)
			require.True(t, ok, "expected NotificationMessage")

			assert.Equal(t, "system", notifMsg.MessageType())
			assert.Equal(t, "notification", notifMsg.Subtype)
			assert.NotEmpty(t, notifMsg.Key)
			assert.NotEmpty(t, notifMsg.Text)
			assert.Equal(t, tt.wantPriority, notifMsg.Priority)
			assert.Equal(t, tt.wantColor, notifMsg.Color)

			if tt.wantTimeoutMS == nil {
				assert.Nil(t, notifMsg.TimeoutMS)
			} else {
				require.NotNil(t, notifMsg.TimeoutMS)
				assert.Equal(t, *tt.wantTimeoutMS, *notifMsg.TimeoutMS)
			}
		})
	}
}

func TestParseMessagePluginInstall(t *testing.T) {
	cases := []struct {
		status PluginInstallStatus
		name   string
		err    string
	}{
		{status: PluginInstallStatusStarted},
		{status: PluginInstallStatusInstalled, name: "marketplace-foo"},
		{status: PluginInstallStatusFailed, name: "marketplace-bar", err: "checksum mismatch"},
		{status: PluginInstallStatusCompleted},
	}

	for i, c := range cases {
		t.Run(string(c.status), func(t *testing.T) {
			body := `"status":"` + string(c.status) + `"`
			if c.name != "" {
				body += `,"name":"` + c.name + `"`
			}
			if c.err != "" {
				body += `,"error":"` + c.err + `"`
			}
			input := `{
				"type": "system",
				"subtype": "plugin_install",
				` + body + `,
				"uuid": "550e8400-e29b-41d4-a716-44665544027` + string(rune('0'+i)) + `",
				"session_id": "sess_misc_008"
			}`

			msg, err := ParseMessage([]byte(input))
			require.NoError(t, err)

			plugMsg, ok := msg.(PluginInstallMessage)
			require.True(t, ok, "expected PluginInstallMessage")

			assert.Equal(t, "system", plugMsg.MessageType())
			assert.Equal(t, "plugin_install", plugMsg.Subtype)
			assert.Equal(t, c.status, plugMsg.Status)
			assert.Equal(t, c.name, plugMsg.Name)
			assert.Equal(t, c.err, plugMsg.Error)
		})
	}
}

func TestParseMessageSessionStateChanged(t *testing.T) {
	for _, state := range []SessionState{SessionStateIdle, SessionStateRunning, SessionStateRequiresAction} {
		t.Run(string(state), func(t *testing.T) {
			input := `{
				"type": "system",
				"subtype": "session_state_changed",
				"state": "` + string(state) + `",
				"uuid": "550e8400-e29b-41d4-a716-446655440290",
				"session_id": "sess_misc_009"
			}`

			msg, err := ParseMessage([]byte(input))
			require.NoError(t, err)

			stateMsg, ok := msg.(SessionStateChangedMessage)
			require.True(t, ok, "expected SessionStateChangedMessage")

			assert.Equal(t, "system", stateMsg.MessageType())
			assert.Equal(t, "session_state_changed", stateMsg.Subtype)
			assert.Equal(t, state, stateMsg.State)
		})
	}
}

func TestParseMessageStatus(t *testing.T) {
	tests := []struct {
		name               string
		input              string
		wantStatusNil      bool
		wantStatus         SDKStatusValue
		wantPermissionMode PermissionMode
		wantCompactResult  CompactResult
		wantCompactError   string
	}{
		{
			name: "compacting with permission mode",
			input: `{
				"type": "system",
				"subtype": "status",
				"status": "compacting",
				"permissionMode": "acceptEdits",
				"uuid": "550e8400-e29b-41d4-a716-4466554402A0",
				"session_id": "sess_misc_010"
			}`,
			wantStatus:         SDKStatusCompacting,
			wantPermissionMode: PermissionModeAcceptEdits,
		},
		{
			name: "null status",
			input: `{
				"type": "system",
				"subtype": "status",
				"status": null,
				"uuid": "550e8400-e29b-41d4-a716-4466554402A1",
				"session_id": "sess_misc_010"
			}`,
			wantStatusNil: true,
		},
		{
			name: "compact failed with error",
			input: `{
				"type": "system",
				"subtype": "status",
				"status": "requesting",
				"compact_result": "failed",
				"compact_error": "context too large",
				"uuid": "550e8400-e29b-41d4-a716-4466554402A2",
				"session_id": "sess_misc_010"
			}`,
			wantStatus:        SDKStatusRequesting,
			wantCompactResult: CompactResultFailed,
			wantCompactError:  "context too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseMessage([]byte(tt.input))
			require.NoError(t, err)

			statusMsg, ok := msg.(StatusMessage)
			require.True(t, ok, "expected StatusMessage")

			assert.Equal(t, "system", statusMsg.MessageType())
			assert.Equal(t, "status", statusMsg.Subtype)

			if tt.wantStatusNil {
				assert.Nil(t, statusMsg.Status)
			} else {
				require.NotNil(t, statusMsg.Status)
				assert.Equal(t, tt.wantStatus, *statusMsg.Status)
			}
			assert.Equal(t, tt.wantPermissionMode, statusMsg.PermissionMode)
			assert.Equal(t, tt.wantCompactResult, statusMsg.CompactResult)
			assert.Equal(t, tt.wantCompactError, statusMsg.CompactError)
		})
	}
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

func intPtr(i int) *int { return &i }

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
