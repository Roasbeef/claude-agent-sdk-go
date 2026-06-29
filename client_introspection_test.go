package claudeagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storeInitResponse(p *Protocol, init *SDKControlInitializeResponse) {
	p.initResponse.Store(init)
	p.initialized.Store(true)
}

func canonicalInitResponse() *SDKControlInitializeResponse {
	return &SDKControlInitializeResponse{
		Commands: []SlashCommand{
			{Name: "help", Description: "show help", ArgumentHint: ""},
			{Name: "model", Description: "switch model", ArgumentHint: "<name>"},
		},
		Agents: []AgentInfo{
			{Name: "Explore", Description: "exploratory agent", Model: "haiku"},
		},
		OutputStyle:           "default",
		AvailableOutputStyles: []string{"default", "concise"},
		Models: []ModelInfo{
			{Value: "claude-sonnet-4-5-20250929", DisplayName: "Sonnet 4.5", Description: "balanced"},
		},
		Account: AccountInfo{
			Email:            "user@example.com",
			Organization:     "ACME",
			SubscriptionType: "pro",
			TokenSource:      "oauth",
			APIKeySource:     "user",
			APIProvider:      APIProviderFirstParty,
		},
		FastModeState: "off",
	}
}

func TestStreamInitializationResultClonesCachedInit(t *testing.T) {
	stream, _, protocol := newStreamControlTest(nil)
	storeInitResponse(protocol, canonicalInitResponse())

	got, err := stream.InitializationResult()
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "default", got.OutputStyle)
	require.Len(t, got.Commands, 2)
	assert.Equal(t, "help", got.Commands[0].Name)
	assert.Equal(t, APIProviderFirstParty, got.Account.APIProvider)

	// Mutate the returned slices/structs; second call must not see the mutation.
	got.Commands[0].Name = "mutated"
	got.Models = append(got.Models, ModelInfo{Value: "extra"})

	again, err := stream.InitializationResult()
	require.NoError(t, err)
	assert.Equal(t, "help", again.Commands[0].Name)
	assert.Len(t, again.Models, 1)
}

func TestStreamSupportedCommandsReadsCachedInit(t *testing.T) {
	stream, _, protocol := newStreamControlTest(nil)
	storeInitResponse(protocol, canonicalInitResponse())

	cmds, err := stream.SupportedCommands(context.Background())
	require.NoError(t, err)
	require.Len(t, cmds, 2)
	assert.Equal(t, "model", cmds[1].Name)

	cmds[0].Name = "mutated"
	again, err := stream.SupportedCommands(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "help", again[0].Name)
}

func TestStreamSupportedModelsReadsCachedInit(t *testing.T) {
	stream, _, protocol := newStreamControlTest(nil)
	storeInitResponse(protocol, canonicalInitResponse())

	models, err := stream.SupportedModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "Sonnet 4.5", models[0].DisplayName)

	models[0].Value = "mutated"
	again, err := stream.SupportedModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-4-5-20250929", again[0].Value)
}

func TestStreamSupportedAgentsReadsCachedInit(t *testing.T) {
	stream, _, protocol := newStreamControlTest(nil)
	storeInitResponse(protocol, canonicalInitResponse())

	agents, err := stream.SupportedAgents(context.Background())
	require.NoError(t, err)
	require.Len(t, agents, 1)
	assert.Equal(t, "Explore", agents[0].Name)

	agents[0].Name = "mutated"
	again, err := stream.SupportedAgents(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Explore", again[0].Name)
}

func TestStreamAccountInfoReadsCachedInit(t *testing.T) {
	stream, _, protocol := newStreamControlTest(nil)
	storeInitResponse(protocol, canonicalInitResponse())

	acct, err := stream.AccountInfo(context.Background())
	require.NoError(t, err)
	require.NotNil(t, acct)
	assert.Equal(t, "user@example.com", acct.Email)
	assert.Equal(t, APIProviderFirstParty, acct.APIProvider)

	acct.Email = "mutated@example.com"
	again, err := stream.AccountInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "user@example.com", again.Email)
}

func TestStreamCachedReadersNotInitialized(t *testing.T) {
	stream, _, _ := newStreamControlTest(nil)

	_, err := stream.InitializationResult()
	assert.True(t, errors.Is(err, ErrNotInitialized))

	_, err = stream.SupportedCommands(context.Background())
	assert.True(t, errors.Is(err, ErrNotInitialized))

	_, err = stream.SupportedModels(context.Background())
	assert.True(t, errors.Is(err, ErrNotInitialized))

	_, err = stream.SupportedAgents(context.Background())
	assert.True(t, errors.Is(err, ErrNotInitialized))

	_, err = stream.AccountInfo(context.Background())
	assert.True(t, errors.Is(err, ErrNotInitialized))
}

func TestStreamMcpServerStatusParsesMcpServersField(t *testing.T) {
	stream, transport, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response: map[string]interface{}{
					"mcpServers": []interface{}{
						map[string]interface{}{
							"name":   "foo",
							"status": "connected",
							"serverInfo": map[string]interface{}{
								"name":    "foo",
								"version": "1.0",
							},
						},
						map[string]interface{}{
							"name":   "bar",
							"status": "needs-auth",
						},
					},
				},
			},
		}
	})

	got, err := stream.McpServerStatus(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "foo", got[0].Name)
	assert.Equal(t, McpServerStateConnected, got[0].Status)
	require.NotNil(t, got[0].ServerInfo)
	assert.Equal(t, "1.0", got[0].ServerInfo.Version)
	assert.Equal(t, McpServerStateNeedsAuth, got[1].Status)
	assert.Nil(t, got[1].ServerInfo)

	_, generic := decodeWrittenSDKControlRequest(t, transport)
	body := genericRequestBody(t, generic)
	assert.Equal(t, "mcp_status", body["subtype"])
}

func TestStreamMcpServerStatusMissingFieldErrors(t *testing.T) {
	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response: map[string]interface{}{
					"servers": []interface{}{},
				},
			},
		}
	})

	_, err := stream.McpServerStatus(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcpServers")
}

func TestStreamGetContextUsageParsesCanonicalPayload(t *testing.T) {
	payload := map[string]interface{}{
		"categories": []interface{}{
			map[string]interface{}{"name": "system", "tokens": 1200, "color": "#fff"},
			map[string]interface{}{"name": "tools", "tokens": 800, "color": "#aaa", "isDeferred": true},
		},
		"totalTokens":  2000,
		"maxTokens":    200000,
		"rawMaxTokens": 200000,
		"percentage":   1.0,
		"gridRows": []interface{}{
			[]interface{}{
				map[string]interface{}{
					"color": "#fff", "isFilled": true, "categoryName": "system",
					"tokens": 1200, "percentage": 0.6, "squareFullness": 1.0,
				},
			},
		},
		"model":       "claude-sonnet-4-5-20250929",
		"memoryFiles": []interface{}{map[string]interface{}{"path": "/CLAUDE.md", "type": "project", "tokens": 100}},
		"mcpTools": []interface{}{
			map[string]interface{}{"name": "search", "serverName": "foo", "tokens": 50, "isLoaded": true},
		},
		"agents": []interface{}{
			map[string]interface{}{"agentType": "Explore", "source": "builtin", "tokens": 30},
		},
		"slashCommands": map[string]interface{}{
			"totalCommands": 10, "includedCommands": 8, "tokens": 200,
		},
		"deferredBuiltinTools": []interface{}{
			map[string]interface{}{"name": "Write", "tokens": 40, "isLoaded": false},
		},
		"systemTools": []interface{}{
			map[string]interface{}{"name": "Read", "tokens": 25},
		},
		"systemPromptSections": []interface{}{
			map[string]interface{}{"name": "preamble", "tokens": 75},
		},
		"skills": map[string]interface{}{
			"totalSkills":    5,
			"includedSkills": 3,
			"tokens":         60,
			"skillFrontmatter": []interface{}{
				map[string]interface{}{"name": "init", "source": "user", "tokens": 20},
			},
		},
		"autoCompactThreshold": 0.85,
		"messageBreakdown": map[string]interface{}{
			"toolCallTokens":          12,
			"toolResultTokens":        18,
			"attachmentTokens":        4,
			"assistantMessageTokens":  300,
			"userMessageTokens":       200,
			"redirectedContextTokens": 0,
			"unattributedTokens":      6,
			"toolCallsByType": []interface{}{
				map[string]interface{}{
					"name": "Read", "callTokens": 4, "resultTokens": 8,
				},
			},
			"attachmentsByType": []interface{}{
				map[string]interface{}{"name": "image", "tokens": 4},
			},
		},
		"isAutoCompactEnabled": true,
		"apiUsage": map[string]interface{}{
			"input_tokens": 100, "output_tokens": 50,
			"cache_creation_input_tokens": 0, "cache_read_input_tokens": 0,
		},
	}

	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response:  payload,
			},
		}
	})

	got, err := stream.GetContextUsage(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 2000, got.TotalTokens)
	assert.Equal(t, "claude-sonnet-4-5-20250929", got.Model)
	require.Len(t, got.Categories, 2)
	assert.True(t, got.Categories[1].IsDeferred)
	require.NotNil(t, got.SlashCommands)
	assert.Equal(t, 8, got.SlashCommands.IncludedCommands)
	require.NotNil(t, got.APIUsage)
	assert.Equal(t, 100, got.APIUsage.InputTokens)
	assert.True(t, got.IsAutoCompactEnabled)

	require.Len(t, got.DeferredBuiltinTools, 1)
	assert.Equal(t, "Write", got.DeferredBuiltinTools[0].Name)
	assert.False(t, got.DeferredBuiltinTools[0].IsLoaded)

	require.Len(t, got.SystemTools, 1)
	assert.Equal(t, "Read", got.SystemTools[0].Name)

	require.Len(t, got.SystemPromptSections, 1)
	assert.Equal(t, "preamble", got.SystemPromptSections[0].Name)

	require.NotNil(t, got.Skills)
	assert.Equal(t, 5, got.Skills.TotalSkills)
	require.Len(t, got.Skills.SkillFrontmatter, 1)
	assert.Equal(t, "init", got.Skills.SkillFrontmatter[0].Name)

	require.NotNil(t, got.AutoCompactThreshold)
	assert.InDelta(t, 0.85, *got.AutoCompactThreshold, 1e-9)

	require.NotNil(t, got.MessageBreakdown)
	assert.Equal(t, 300, got.MessageBreakdown.AssistantMessageTokens)
	require.Len(t, got.MessageBreakdown.ToolCallsByType, 1)
	assert.Equal(t, 8, got.MessageBreakdown.ToolCallsByType[0].ResultTokens)
	require.Len(t, got.MessageBreakdown.AttachmentsByType, 1)
	assert.Equal(t, "image", got.MessageBreakdown.AttachmentsByType[0].Name)
}

func TestStreamGetContextUsageApiUsageNullable(t *testing.T) {
	payload := map[string]interface{}{
		"categories":           []interface{}{},
		"totalTokens":          0,
		"maxTokens":            200000,
		"rawMaxTokens":         200000,
		"percentage":           0.0,
		"gridRows":             []interface{}{},
		"model":                "claude-sonnet-4-5-20250929",
		"memoryFiles":          []interface{}{},
		"mcpTools":             []interface{}{},
		"agents":               []interface{}{},
		"isAutoCompactEnabled": false,
		"apiUsage":             nil,
	}

	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response:  payload,
			},
		}
	})

	got, err := stream.GetContextUsage(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.APIUsage)
}

func TestStreamGetUsageExperimentalParsesCanonicalPayload(t *testing.T) {
	payload := map[string]interface{}{
		"session": map[string]interface{}{
			"total_cost_usd":        1.25,
			"total_api_duration_ms": 4200,
			"total_duration_ms":     9000,
			"total_lines_added":     120,
			"total_lines_removed":   40,
			"model_usage": map[string]interface{}{
				"claude-opus-4-8": map[string]interface{}{
					"inputTokens": 1000, "outputTokens": 500, "costUSD": 1.25,
				},
			},
		},
		"subscription_type":     "max",
		"rate_limits_available": true,
		"rate_limits": map[string]interface{}{
			"five_hour": map[string]interface{}{
				"utilization": 42.5, "resets_at": "2026-06-15T20:00:00Z",
			},
			"seven_day": nil,
			"model_scoped": []interface{}{
				map[string]interface{}{
					"display_name": "Fable", "utilization": 60.0,
					"resets_at": "2026-06-22T00:00:00Z",
				},
				map[string]interface{}{
					"display_name": "Opus", "utilization": nil, "resets_at": nil,
				},
			},
			"extra_usage": map[string]interface{}{
				"is_enabled": true, "monthly_limit": 50.0,
				"used_credits": 12.5, "utilization": 25.0, "currency": "USD",
			},
		},
		"behaviors": map[string]interface{}{
			"day": map[string]interface{}{
				"request_count": 10, "session_count": 3,
				"behaviors": []interface{}{
					map[string]interface{}{"key": "cache_miss", "pct": 30.0, "count": 3},
				},
				"agents":      []interface{}{map[string]interface{}{"name": "Explore", "pct": 20.0}},
				"skills":      []interface{}{},
				"plugins":     []interface{}{},
				"mcp_servers": []interface{}{map[string]interface{}{"name": "github", "pct": 5.0}},
			},
			"week": map[string]interface{}{
				"request_count": 70, "session_count": 14,
				"behaviors":   []interface{}{},
				"agents":      []interface{}{},
				"skills":      []interface{}{},
				"plugins":     []interface{}{},
				"mcp_servers": []interface{}{},
			},
		},
	}

	var gotSubtype string
	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		gotSubtype = req.Request.Subtype
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response:  payload,
			},
		}
	})

	got, err := stream.GetUsageExperimental(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "get_usage", gotSubtype)

	assert.InDelta(t, 1.25, got.Session.TotalCostUSD, 1e-9)
	assert.Equal(t, 120, got.Session.TotalLinesAdded)
	require.Contains(t, got.Session.ModelUsage, "claude-opus-4-8")
	assert.Equal(t, 1000, got.Session.ModelUsage["claude-opus-4-8"].InputTokens)

	require.NotNil(t, got.SubscriptionType)
	assert.Equal(t, "max", *got.SubscriptionType)
	assert.True(t, got.RateLimitsAvailable)

	require.NotNil(t, got.RateLimits)
	require.NotNil(t, got.RateLimits.FiveHour)
	require.NotNil(t, got.RateLimits.FiveHour.Utilization)
	assert.InDelta(t, 42.5, *got.RateLimits.FiveHour.Utilization, 1e-9)
	assert.Nil(t, got.RateLimits.SevenDay) // present-but-null window
	require.Len(t, got.RateLimits.ModelScoped, 2)
	assert.Equal(t, "Fable", got.RateLimits.ModelScoped[0].DisplayName)
	require.NotNil(t, got.RateLimits.ModelScoped[0].Utilization)
	assert.InDelta(t, 60.0, *got.RateLimits.ModelScoped[0].Utilization, 1e-9)
	require.NotNil(t, got.RateLimits.ModelScoped[0].ResetsAt)
	assert.Equal(t, "Opus", got.RateLimits.ModelScoped[1].DisplayName)
	assert.Nil(t, got.RateLimits.ModelScoped[1].Utilization) // present-but-null
	assert.Nil(t, got.RateLimits.ModelScoped[1].ResetsAt)
	require.NotNil(t, got.RateLimits.ExtraUsage)
	assert.True(t, got.RateLimits.ExtraUsage.IsEnabled)
	require.NotNil(t, got.RateLimits.ExtraUsage.Currency)
	assert.Equal(t, "USD", *got.RateLimits.ExtraUsage.Currency)

	require.NotNil(t, got.Behaviors)
	assert.Equal(t, 10, got.Behaviors.Day.RequestCount)
	require.Len(t, got.Behaviors.Day.Behaviors, 1)
	assert.Equal(t, "cache_miss", got.Behaviors.Day.Behaviors[0].Key)
	assert.Equal(t, 3, got.Behaviors.Day.Behaviors[0].Count)
	require.Len(t, got.Behaviors.Day.Agents, 1)
	assert.Equal(t, "Explore", got.Behaviors.Day.Agents[0].Name)
	assert.Equal(t, 70, got.Behaviors.Week.RequestCount)
}

func TestStreamGetUsageExperimentalNullables(t *testing.T) {
	payload := map[string]interface{}{
		"session": map[string]interface{}{
			"total_cost_usd": 0.0, "total_api_duration_ms": 0,
			"total_duration_ms": 0, "total_lines_added": 0,
			"total_lines_removed": 0, "model_usage": map[string]interface{}{},
		},
		"subscription_type":     nil,
		"rate_limits_available": false,
		"rate_limits":           nil,
		"behaviors":             nil,
	}

	stream, _, _ := newStreamControlTest(func(req SDKControlRequest) SDKControlResponse {
		return SDKControlResponse{
			Type: "control_response",
			Response: SDKControlResponseBody{
				Subtype:   "success",
				RequestID: req.RequestID,
				Response:  payload,
			},
		}
	})

	got, err := stream.GetUsageExperimental(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.SubscriptionType)
	assert.False(t, got.RateLimitsAvailable)
	assert.Nil(t, got.RateLimits)
	assert.Nil(t, got.Behaviors)
}

// Sanity-check that AccountInfo with apiProvider round-trips through JSON.
func TestAccountInfoAPIProviderJSON(t *testing.T) {
	tests := []struct {
		name     string
		provider APIProvider
		wantJSON string
	}{
		{
			name:     "bedrock",
			provider: APIProviderBedrock,
			wantJSON: `"apiProvider":"bedrock"`,
		},
		{
			name:     "gateway",
			provider: APIProviderGateway,
			wantJSON: `"apiProvider":"gateway"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := AccountInfo{Email: "x@y", APIProvider: tt.provider}
			bytes, err := json.Marshal(in)
			require.NoError(t, err)
			assert.Contains(t, string(bytes), tt.wantJSON)

			var out AccountInfo
			require.NoError(t, json.Unmarshal(bytes, &out))
			assert.Equal(t, tt.provider, out.APIProvider)
		})
	}
}

func TestAccountInfoAPIProviderGatewayUnmarshal(t *testing.T) {
	var acct AccountInfo
	require.NoError(t, json.Unmarshal([]byte(`{"apiProvider":"gateway","apiKeySource":"oauth"}`), &acct))

	assert.Equal(t, APIProviderGateway, acct.APIProvider)
	assert.Equal(t, "oauth", acct.APIKeySource)
}
