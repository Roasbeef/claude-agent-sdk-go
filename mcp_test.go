package claudeagent

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPServerConfigStdio(t *testing.T) {
	config := MCPServerConfig{
		Type:    "stdio",
		Command: "my-server",
		Args:    []string{"--port", "8080"},
	}

	assert.Equal(t, "stdio", config.Type)
	assert.Equal(t, "my-server", config.Command)
	assert.Equal(t, []string{"--port", "8080"}, config.Args)
}

func TestMCPServerConfigSocket(t *testing.T) {
	config := MCPServerConfig{
		Type:    "socket",
		Address: "localhost:9000",
	}

	assert.Equal(t, "socket", config.Type)
	assert.Equal(t, "localhost:9000", config.Address)
}

func TestMCPServerConfigEnv(t *testing.T) {
	config := MCPServerConfig{
		Type:    "stdio",
		Command: "my-server",
		Env: map[string]string{
			"API_KEY": "secret",
			"DEBUG":   "true",
		},
	}

	assert.Equal(t, "secret", config.Env["API_KEY"])
	assert.Equal(t, "true", config.Env["DEBUG"])
}

func TestWithMCPServers(t *testing.T) {
	servers := map[string]MCPServerConfig{
		"server1": {
			Type:    "stdio",
			Command: "cmd1",
		},
		"server2": {
			Type:    "socket",
			Address: "localhost:8080",
		},
	}

	opts := NewOptions()
	WithMCPServers(servers)(opts)

	assert.Len(t, opts.MCPServers, 2)
	assert.Equal(t, "cmd1", opts.MCPServers["server1"].Command)
	assert.Equal(t, "localhost:8080", opts.MCPServers["server2"].Address)
}

// Test the generics-based MCP server API.

func TestCreateMcpServer(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{
		Name:    "test-server",
		Version: "2.0.0",
	})

	assert.Equal(t, "test-server", server.Name())
	assert.Equal(t, "2.0.0", server.Version())
	assert.Empty(t, server.ToolNames())
}

func TestCreateMcpServerDefaultVersion(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{
		Name: "test-server",
	})

	assert.Equal(t, "1.0.0", server.Version())
}

// AddNumbersArgs is a test type for generics tests.
type AddNumbersArgs struct {
	A int `json:"a"`
	B int `json:"b"`
}

// AddNumbersResult is a test type for typed responses.
type AddNumbersResult struct {
	Sum int `json:"sum"`
}

func TestCreateMcpServerWithTools(t *testing.T) {
	// Test creating a server with tools in options.
	server := CreateMcpServer(McpServerOptions{
		Name: "calculator",
		Tools: []ToolRegistrar{
			Tool("add", "Add two numbers",
				func(ctx context.Context, args AddNumbersArgs) (ToolResult, error) {
					return TextResult(fmt.Sprintf("%d", args.A+args.B)), nil
				},
			),
			Tool("multiply", "Multiply two numbers",
				func(ctx context.Context, args AddNumbersArgs) (ToolResult, error) {
					return TextResult(fmt.Sprintf("%d", args.A*args.B)), nil
				},
			),
		},
	})

	assert.Len(t, server.ToolNames(), 2)
	assert.Contains(t, server.ToolNames(), "add")
	assert.Contains(t, server.ToolNames(), "multiply")
}

func TestToolWithResponse(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{
		Name: "calculator",
		Tools: []ToolRegistrar{
			ToolWithResponse("add", "Add two numbers",
				func(ctx context.Context, args AddNumbersArgs) (AddNumbersResult, error) {
					return AddNumbersResult{Sum: args.A + args.B}, nil
				},
			),
		},
	})

	ctx := context.Background()
	result, err := server.CallTool(ctx, "add", json.RawMessage(`{"a": 5, "b": 3}`))

	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)

	// The response should be JSON marshaled.
	assert.Contains(t, result.Content[0].Text, `"sum":8`)
}

func TestToolWithResponseError(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{
		Name: "failing",
		Tools: []ToolRegistrar{
			ToolWithResponse("fail", "Always fails",
				func(ctx context.Context, args AddNumbersArgs) (AddNumbersResult, error) {
					return AddNumbersResult{}, fmt.Errorf("intentional error")
				},
			),
		},
	})

	ctx := context.Background()
	result, err := server.CallTool(ctx, "fail", json.RawMessage(`{"a": 1, "b": 2}`))

	require.NoError(t, err) // No Go error.
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "intentional error")
}

func TestToolWithSchema(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"a": map[string]interface{}{"type": "integer"},
			"b": map[string]interface{}{"type": "integer"},
		},
		"required": []string{"a", "b"},
	}

	server := CreateMcpServer(McpServerOptions{
		Name: "calculator",
		Tools: []ToolRegistrar{
			ToolWithSchema("add", "Add two numbers", schema,
				func(ctx context.Context, args AddNumbersArgs) (ToolResult, error) {
					return TextResult(fmt.Sprintf("%d", args.A+args.B)), nil
				},
			),
		},
	})

	defs := server.ToolDefs()
	require.Len(t, defs, 1)
	assert.NotNil(t, defs[0].InputSchema)
}

func TestAddTool(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{Name: "calculator"})

	AddTool(server, ToolDef{
		Name:        "add",
		Description: "Add two numbers",
	}, func(ctx context.Context, args AddNumbersArgs) (ToolResult, error) {
		return TextResult("result"), nil
	})

	assert.Contains(t, server.ToolNames(), "add")
	assert.Len(t, server.ToolDefs(), 1)
	assert.Equal(t, "add", server.ToolDefs()[0].Name)
	assert.Equal(t, "Add two numbers", server.ToolDefs()[0].Description)
}

func TestAddToolWithResponse(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{Name: "calculator"})

	AddToolWithResponse(server, ToolDef{
		Name:        "add",
		Description: "Add two numbers",
	}, func(ctx context.Context, args AddNumbersArgs) (AddNumbersResult, error) {
		return AddNumbersResult{Sum: args.A + args.B}, nil
	})

	ctx := context.Background()
	result, err := server.CallTool(ctx, "add", json.RawMessage(`{"a": 10, "b": 20}`))

	require.NoError(t, err)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, `"sum":30`)
}

func TestAddToolUntyped(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{Name: "dynamic"})

	AddToolUntyped(server, ToolDef{
		Name:        "echo",
		Description: "Echo back input",
	}, func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
		return TextResult(string(args)), nil
	})

	assert.Contains(t, server.ToolNames(), "echo")
}

func TestCallTool(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{Name: "calculator"})

	AddTool(server, ToolDef{
		Name:        "add",
		Description: "Add two numbers",
	}, func(ctx context.Context, args AddNumbersArgs) (ToolResult, error) {
		return TextResult(fmt.Sprintf("%d", args.A+args.B)), nil
	})

	ctx := context.Background()
	result, err := server.CallTool(ctx, "add", json.RawMessage(`{"a": 5, "b": 3}`))

	require.NoError(t, err)
	assert.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "8", result.Content[0].Text)
}

func TestCallToolNotFound(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{Name: "empty"})

	ctx := context.Background()
	_, err := server.CallTool(ctx, "nonexistent", json.RawMessage(`{}`))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool not found")
}

func TestCallToolInvalidArgs(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{Name: "calculator"})

	AddTool(server, ToolDef{
		Name:        "add",
		Description: "Add two numbers",
	}, func(ctx context.Context, args AddNumbersArgs) (ToolResult, error) {
		return TextResult("should not reach"), nil
	})

	ctx := context.Background()

	// Invalid JSON - should return error result.
	result, err := server.CallTool(ctx, "add", json.RawMessage(`{invalid`))

	require.NoError(t, err) // No Go error, but IsError should be true.
	assert.True(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "invalid arguments")
}

func TestToolResultHelpers(t *testing.T) {
	t.Run("TextContent", func(t *testing.T) {
		content := TextContent("hello")
		assert.Equal(t, "text", content.Type)
		assert.Equal(t, "hello", content.Text)
	})

	t.Run("ResourceContent", func(t *testing.T) {
		content := ResourceContent("file://test.txt")
		assert.Equal(t, "resource", content.Type)
		assert.Equal(t, "file://test.txt", content.Resource)
	})

	t.Run("TextResult", func(t *testing.T) {
		result := TextResult("success message")
		assert.False(t, result.IsError)
		assert.Len(t, result.Content, 1)
		assert.Equal(t, "success message", result.Content[0].Text)
	})

	t.Run("ErrorResult", func(t *testing.T) {
		result := ErrorResult("error message")
		assert.True(t, result.IsError)
		assert.Len(t, result.Content, 1)
		assert.Equal(t, "error message", result.Content[0].Text)
	})

	t.Run("ResourceResult", func(t *testing.T) {
		result := ResourceResult("file://path")
		assert.False(t, result.IsError)
		assert.Len(t, result.Content, 1)
		assert.Equal(t, "file://path", result.Content[0].Resource)
	})

	t.Run("MultiContentResult", func(t *testing.T) {
		result := MultiContentResult(
			TextContent("first"),
			TextContent("second"),
		)
		assert.False(t, result.IsError)
		assert.Len(t, result.Content, 2)
	})
}

func TestToolResultJSON(t *testing.T) {
	result := ToolResult{
		Content: []ToolContent{
			{Type: "text", Text: "result"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	assert.NoError(t, err)
	assert.Contains(t, string(data), `"content"`)

	// Verify roundtrip.
	var decoded ToolResult
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Len(t, decoded.Content, 1)
	assert.Equal(t, "result", decoded.Content[0].Text)
}

func TestWithMcpServer(t *testing.T) {
	server := CreateMcpServer(McpServerOptions{Name: "calculator"})

	AddTool(server, ToolDef{
		Name:        "add",
		Description: "Add two numbers",
	}, func(ctx context.Context, args AddNumbersArgs) (ToolResult, error) {
		return TextResult("8"), nil
	})

	opts := NewOptions()
	WithMcpServer("calculator", server)(opts)

	assert.Len(t, opts.SDKMcpServers, 1)
	assert.Equal(t, server, opts.SDKMcpServers["calculator"])
}

func TestWithMcpServerMultiple(t *testing.T) {
	server1 := CreateMcpServer(McpServerOptions{Name: "calculator"})
	server2 := CreateMcpServer(McpServerOptions{Name: "converter"})

	opts := NewOptions()
	WithMcpServer("calc", server1)(opts)
	WithMcpServer("conv", server2)(opts)

	assert.Len(t, opts.SDKMcpServers, 2)
	assert.Equal(t, server1, opts.SDKMcpServers["calc"])
	assert.Equal(t, server2, opts.SDKMcpServers["conv"])
}

func TestServerAddToolMethod(t *testing.T) {
	// Test the method-based AddTool with untyped handler.
	server := CreateMcpServer(McpServerOptions{Name: "test"}).
		AddTool("echo", "Echo input", func(ctx context.Context, args json.RawMessage) (ToolResult, error) {
			return TextResult(string(args)), nil
		})

	assert.Contains(t, server.ToolNames(), "echo")

	ctx := context.Background()
	result, err := server.CallTool(ctx, "echo", json.RawMessage(`"hello"`))

	require.NoError(t, err)
	assert.Equal(t, `"hello"`, result.Content[0].Text)
}
