// Example MCP server demonstrating how to create tools that Claude can use.
//
// This server uses the official github.com/modelcontextprotocol/go-sdk
// and provides simple tools for testing MCP integration with the Claude Agent SDK.
//
// Usage:
//
//	go build -o example-mcp-server ./cmd/example-mcp-server
//	# Then configure in your client:
//	# WithMCPServers(map[string]MCPServerConfig{
//	#     "example": {Command: "./example-mcp-server"},
//	# })
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AddNumbersArgs is the input schema for the add_numbers tool.
type AddNumbersArgs struct {
	A int `json:"a" jsonschema:"First number to add"`
	B int `json:"b" jsonschema:"Second number to add"`
}

// EchoArgs is the input schema for the echo tool.
type EchoArgs struct {
	Message string `json:"message" jsonschema:"Message to echo back"`
}

// ReverseArgs is the input schema for the reverse tool.
type ReverseArgs struct {
	Text string `json:"text" jsonschema:"Text to reverse"`
}

func main() {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "example-mcp-server",
			Version: "1.0.0",
		},
		nil,
	)

	// Tool: add_numbers - Adds two integers.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_numbers",
		Description: "Add two numbers together and return the sum",
	}, func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		args AddNumbersArgs,
	) (*mcp.CallToolResult, any, error) {
		result := args.A + args.B
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("%d", result)},
			},
		}, nil, nil
	})

	// Tool: echo - Echoes back the input message.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "Echo back the provided message",
	}, func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		args EchoArgs,
	) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: args.Message},
			},
		}, nil, nil
	})

	// Tool: reverse - Reverses a string.
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reverse",
		Description: "Reverse the characters in a string",
	}, func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		args ReverseArgs,
	) (*mcp.CallToolResult, any, error) {
		runes := []rune(args.Text)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(runes)},
			},
		}, nil, nil
	})

	// Tool: concat - Concatenates multiple strings.
	type ConcatArgs struct {
		Parts     []string `json:"parts" jsonschema:"Strings to concatenate"`
		Separator string   `json:"separator,omitempty" jsonschema:"Separator between parts (default empty)"`
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        "concat",
		Description: "Concatenate multiple strings with an optional separator",
	}, func(
		ctx context.Context,
		req *mcp.CallToolRequest,
		args ConcatArgs,
	) (*mcp.CallToolResult, any, error) {
		result := strings.Join(args.Parts, args.Separator)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: result},
			},
		}, nil, nil
	})

	// Run the server on stdio transport.
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
