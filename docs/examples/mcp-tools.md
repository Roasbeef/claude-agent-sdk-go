# MCP Tools

This guide explains how to integrate custom tools with Claude using the Model
Context Protocol (MCP). Tools let Claude interact with external systems, APIs,
and data sources during conversations.

## Overview

The SDK supports two types of MCP servers:

| Type | Description | Use Case |
|------|-------------|----------|
| **In-Process (SDK)** | Tools run in your Go process | Custom Go functions, no separate binary |
| **Binary (Subprocess)** | External MCP server binary | Existing MCP servers, language-agnostic |

## In-Process MCP Servers

In-process servers run tools directly in your Go application. This is the
simplest approach for custom tools.

### Basic Example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/roasbeef/claude-agent-sdk-go"
)

// Define argument types as structs with JSON tags.
type AddArgs struct {
    A int `json:"a"`
    B int `json:"b"`
}

func main() {
    // Create an MCP server with tools.
    server := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{
        Name:    "calculator",
        Version: "1.0.0",
        Tools: []claudeagent.ToolRegistrar{
            claudeagent.Tool("add", "Add two numbers",
                func(ctx context.Context, args AddArgs) (claudeagent.ToolResult, error) {
                    return claudeagent.TextResult(fmt.Sprintf("%d", args.A+args.B)), nil
                },
            ),
        },
    })

    // Create client with the MCP server.
    client, err := claudeagent.NewClient(
        claudeagent.WithMcpServer("calculator", server),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Use it.
    ctx := context.Background()
    for msg := range client.Query(ctx, "What is 7 + 4?") {
        if m, ok := msg.(claudeagent.AssistantMessage); ok {
            fmt.Println(m.ContentText())
        }
    }
}
```

### Tool Creation Patterns

The SDK provides several ways to create tools, depending on your needs:

#### 1. `Tool[Args]` - Basic Typed Handler

Use when you need typed arguments and return `ToolResult` directly.

```go
type FetchQuoteArgs struct {
    Symbol string `json:"symbol"`
}

claudeagent.Tool("fetch_quote", "Get stock price",
    func(ctx context.Context, args FetchQuoteArgs) (claudeagent.ToolResult, error) {
        price, err := api.GetPrice(args.Symbol)
        if err != nil {
            return claudeagent.ErrorResult(err.Error()), nil
        }
        return claudeagent.TextResult(fmt.Sprintf("$%.2f", price)), nil
    },
)
```

#### 2. `ToolWithResponse[Args, Response]` - Typed Args and Response

Use when you want both typed arguments and a typed response (auto-serialized to
JSON).

```go
type AddArgs struct {
    A int `json:"a"`
    B int `json:"b"`
}

type AddResult struct {
    Sum int `json:"sum"`
}

claudeagent.ToolWithResponse("add", "Add two numbers",
    func(ctx context.Context, args AddArgs) (AddResult, error) {
        return AddResult{Sum: args.A + args.B}, nil
    },
)
```

#### 3. `ToolWithSchema[Args]` - Explicit JSON Schema

Use when you need to provide a custom JSON schema for input validation.

```go
schema := map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "symbol": map[string]interface{}{
            "type":        "string",
            "description": "Stock ticker symbol",
            "pattern":     "^[A-Z]{1,5}$",
        },
    },
    "required": []string{"symbol"},
}

claudeagent.ToolWithSchema("fetch_quote", "Get stock price", schema,
    func(ctx context.Context, args FetchQuoteArgs) (claudeagent.ToolResult, error) {
        // ...
    },
)
```

#### 4. Package-Level Functions

For adding tools to an existing server:

```go
server := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{
    Name: "calculator",
})

// Using AddTool (generic)
claudeagent.AddTool(server, claudeagent.ToolDef{
    Name:        "add",
    Description: "Add two numbers",
}, func(ctx context.Context, args AddArgs) (claudeagent.ToolResult, error) {
    return claudeagent.TextResult(fmt.Sprintf("%d", args.A+args.B)), nil
})

// Using AddToolWithResponse (typed response)
claudeagent.AddToolWithResponse(server, claudeagent.ToolDef{
    Name:        "multiply",
    Description: "Multiply two numbers",
}, func(ctx context.Context, args MultiplyArgs) (MultiplyResult, error) {
    return MultiplyResult{Product: args.A * args.B}, nil
})

// Using AddToolUntyped (raw JSON)
claudeagent.AddToolUntyped(server, claudeagent.ToolDef{
    Name:        "dynamic",
    Description: "Handle dynamic arguments",
    InputSchema: customSchema,
}, func(ctx context.Context, args json.RawMessage) (claudeagent.ToolResult, error) {
    var data map[string]interface{}
    json.Unmarshal(args, &data)
    return claudeagent.TextResult("processed"), nil
})
```

### Result Helpers

The SDK provides helpers for creating tool results:

```go
// Simple text result
claudeagent.TextResult("Success!")

// Error result (shows as error to Claude)
claudeagent.ErrorResult("Invalid input: symbol required")

// Resource result
claudeagent.ResourceResult("file:///path/to/resource")

// Multiple content items
claudeagent.MultiContentResult(
    claudeagent.TextContent("Header"),
    claudeagent.TextContent("Body content"),
    claudeagent.ResourceContent("file:///attachment"),
)
```

### Multiple Tools

Register multiple tools in a single server:

```go
server := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{
    Name:    "trading",
    Version: "1.0.0",
    Tools: []claudeagent.ToolRegistrar{
        claudeagent.Tool("fetch_quote", "Get stock price", fetchQuoteHandler),
        claudeagent.Tool("place_order", "Place a trade", placeOrderHandler),
        claudeagent.Tool("get_portfolio", "Get holdings", getPortfolioHandler),
        claudeagent.ToolWithResponse("calculate_return", "Calculate returns", calcReturnHandler),
    },
})
```

### Method Chaining

You can also chain tool additions:

```go
server := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{Name: "calc"})

// Note: Method chaining requires raw JSON handlers
server.AddTool("add", "Add numbers", rawAddHandler).
    AddTool("sub", "Subtract numbers", rawSubHandler)
```

## Binary MCP Servers

For external MCP server binaries (subprocess-based):

```go
client, _ := claudeagent.NewClient(
    claudeagent.WithMCPServers(map[string]claudeagent.MCPServerConfig{
        "filesystem": {
            Command: "npx",
            Args:    []string{"-y", "@anthropic-ai/mcp-server-filesystem", "/tmp"},
        },
        "custom-server": {
            Command: "/usr/local/bin/my-mcp-server",
            Args:    []string{"--verbose"},
            Env: map[string]string{
                "API_KEY": os.Getenv("MY_API_KEY"),
            },
        },
    }),
)
```

The binary is spawned as a subprocess and communicates via the MCP protocol
over stdio.

## Combining Both Types

You can use both in-process and binary servers together:

```go
// In-process server for custom tools
calcServer := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{
    Name: "calculator",
    Tools: []claudeagent.ToolRegistrar{
        claudeagent.Tool("add", "Add numbers", addHandler),
    },
})

client, _ := claudeagent.NewClient(
    // In-process server
    claudeagent.WithMcpServer("calculator", calcServer),
    // Binary servers
    claudeagent.WithMCPServers(map[string]claudeagent.MCPServerConfig{
        "filesystem": {
            Command: "npx",
            Args:    []string{"-y", "@anthropic-ai/mcp-server-filesystem", "/tmp"},
        },
    }),
)
```

## Error Handling

Return user-friendly error messages that Claude can relay:

```go
func fetchQuoteHandler(ctx context.Context, args FetchQuoteArgs) (claudeagent.ToolResult, error) {
    if args.Symbol == "" {
        return claudeagent.ErrorResult("Symbol is required"), nil
    }

    quote, err := api.GetQuote(args.Symbol)
    if err != nil {
        if errors.Is(err, api.ErrSymbolNotFound) {
            return claudeagent.ErrorResult(
                fmt.Sprintf("Symbol '%s' not found. Please check the ticker.", args.Symbol),
            ), nil
        }
        if errors.Is(err, api.ErrRateLimited) {
            return claudeagent.ErrorResult("Rate limited. Please wait and retry."), nil
        }
        // Generic error
        return claudeagent.ErrorResult("Unable to fetch quote at this time."), nil
    }

    return claudeagent.TextResult(formatQuote(quote)), nil
}
```

**Best practices:**
- Return `ErrorResult` for expected failures (bad input, API errors)
- Return Go `error` only for unexpected failures (panic-worthy situations)
- Provide actionable error messages that help Claude decide what to do next

## Complete Example

Here's a working example with multiple tools:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/roasbeef/claude-agent-sdk-go"
)

type TimezoneArgs struct {
    Timezone string `json:"timezone"`
}

type CalculateArgs struct {
    A  float64 `json:"a"`
    B  float64 `json:"b"`
    Op string  `json:"op"`
}

type CalculateResult struct {
    Result float64 `json:"result"`
    Expr   string  `json:"expression"`
}

func main() {
    server := claudeagent.CreateMcpServer(claudeagent.McpServerOptions{
        Name:    "utilities",
        Version: "1.0.0",
        Tools: []claudeagent.ToolRegistrar{
            claudeagent.Tool("get_time", "Get current time in a timezone",
                func(ctx context.Context, args TimezoneArgs) (claudeagent.ToolResult, error) {
                    tz := args.Timezone
                    if tz == "" {
                        tz = "UTC"
                    }
                    loc, err := time.LoadLocation(tz)
                    if err != nil {
                        return claudeagent.ErrorResult("Invalid timezone: " + tz), nil
                    }
                    return claudeagent.TextResult(time.Now().In(loc).Format(time.RFC1123)), nil
                },
            ),
            claudeagent.ToolWithResponse("calculate", "Perform calculation",
                func(ctx context.Context, args CalculateArgs) (CalculateResult, error) {
                    var result float64
                    switch args.Op {
                    case "add", "+":
                        result = args.A + args.B
                    case "sub", "-":
                        result = args.A - args.B
                    case "mul", "*":
                        result = args.A * args.B
                    case "div", "/":
                        if args.B == 0 {
                            return CalculateResult{}, fmt.Errorf("division by zero")
                        }
                        result = args.A / args.B
                    default:
                        return CalculateResult{}, fmt.Errorf("unknown op: %s", args.Op)
                    }
                    return CalculateResult{
                        Result: result,
                        Expr:   fmt.Sprintf("%.2f %s %.2f = %.2f", args.A, args.Op, args.B, result),
                    }, nil
                },
            ),
        },
    })

    client, err := claudeagent.NewClient(
        claudeagent.WithSystemPrompt("You have access to time and calculation tools."),
        claudeagent.WithMcpServer("utilities", server),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ctx := context.Background()
    for msg := range client.Query(ctx, "What time is it in Tokyo, and what's 15% of 847?") {
        switch m := msg.(type) {
        case claudeagent.AssistantMessage:
            fmt.Println(m.ContentText())
        case claudeagent.ResultMessage:
            fmt.Printf("Cost: $%.4f\n", m.TotalCostUSD)
        }
    }
}
```

## See Also

- [Hooks](hooks.md) - Intercept tool calls before and after execution
- [Permissions](permissions.md) - Control which tools Claude can use
