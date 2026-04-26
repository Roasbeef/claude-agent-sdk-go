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
			APIProvider:      "firstParty",
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
	assert.Equal(t, "firstParty", got.Account.APIProvider)

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
	assert.Equal(t, "firstParty", acct.APIProvider)

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

// Sanity-check that AccountInfo with apiProvider round-trips through JSON.
func TestAccountInfoAPIProviderJSON(t *testing.T) {
	in := AccountInfo{Email: "x@y", APIProvider: "bedrock"}
	bytes, err := json.Marshal(in)
	require.NoError(t, err)
	assert.Contains(t, string(bytes), `"apiProvider":"bedrock"`)

	var out AccountInfo
	require.NoError(t, json.Unmarshal(bytes, &out))
	assert.Equal(t, "bedrock", out.APIProvider)
}
