# Claude Code CLI Protocol

This document describes the stream-json protocol used to communicate with the
Claude Code CLI. Most of this is undocumented in official sources and was
discovered through implementation and testing.

## Launching the CLI

The CLI is invoked with flags that enable machine-readable I/O:

```bash claude --output-format stream-json --input-format stream-json --verbose
```

The `--output-format stream-json` flag causes the CLI to emit line-delimited
JSON on stdout instead of human-readable text. Each line is a complete JSON
object representing a message.

The `--input-format stream-json` flag enables the CLI to accept JSON messages
on stdin. This is required for the control protocol used by SDKs.

The `--verbose` flag is required when using stream-json output format.

Additional flags configure behavior:

```bash
claude \
  --output-format stream-json \
  --input-format stream-json \
  --verbose \
  --model claude-sonnet-4-5-20250929 \
  --system-prompt "You are a helpful assistant." \
  --permission-mode acceptEdits \
  --permission-prompt-tool stdio \
  --setting-sources user,project \
  --no-session-persistence
```

The `--permission-prompt-tool stdio` flag routes permission requests through
the control protocol instead of prompting the user interactively. This allows
SDKs to implement custom permission handling.

The `--setting-sources` flag controls which filesystem settings are loaded. It
accepts a comma-separated list: `user` loads `~/.claude/settings.json`,
`project` loads `.claude/settings.json`, and `local` loads
`.claude/settings.local.json`.

## Message Types

The CLI emits several message types, distinguished by the `type` field:

**assistant** - Claude's response text. Contains a `message` object with
`content` blocks (text, tool_use, thinking).

**user** - Echo of user messages. Contains the original prompt text.

**result** - Completion message with metadata. Includes `total_cost_usd`,
`total_input_tokens`, `total_output_tokens`, and `session_id`.

**system** - System messages from the CLI. Used for initialization confirmation
and error reporting.

**stream** - Incremental updates during generation. The `event` field indicates
the update type: `delta` for new text, `tool_use_start` when Claude begins
using a tool.

**control** - Bidirectional control messages. See the Control Protocol section
below.

Example message flow for a simple query:

```json
{"type":"system","subtype":"init","session_id":"abc123"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Paris is..."}]}}
{"type":"result","total_cost_usd":0.001,"session_id":"abc123"}
```

## Control Protocol

The control protocol enables bidirectional communication between the SDK and
CLI. Control messages have `type: "control"` and are further distinguished by
`subtype`.

### Initialization

When an SDK connects, it sends an initialization message to register hooks and
SDK MCP servers:

```json
{
  "type": "control_request",
  "request": {
    "subtype": "initialize",
    "request_id": "req_1",
    "hooks": {
      "PreToolUse": [{"matcher": "*", "hook_callback_ids": ["hook_0"]}]
    },
    "sdk_mcp_servers": ["calculator", "trading"]
  }
}
```

The `hooks` field registers callbacks for lifecycle events. The `matcher` is a
glob pattern for tool names.

The `sdk_mcp_servers` field declares in-process MCP servers. When Claude needs
to call a tool from one of these servers, the CLI routes the call through the
control protocol instead of spawning a subprocess.

The CLI responds with a confirmation:

```json
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "req_1"
  }
}
```

### Permission Requests

When the SDK is configured with `--permission-prompt-tool stdio`, the CLI sends
permission requests instead of prompting the user:

```json
{
  "type": "sdk_control_request",
  "request": {
    "subtype": "permission",
    "request_id": "perm_1",
    "tool_name": "Bash",
    "tool_input": {"command": "rm -rf /tmp/test"}
  }
}
```

The SDK responds with an allow or deny decision:

```json
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "perm_1",
    "response": {
      "behavior": "allow"
    }
  }
}
```

For denials, include a reason:

```json
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "perm_1",
    "response": {
      "behavior": "deny",
      "message": "Destructive commands are not permitted"
    }
  }
}
```

### MCP Tool Calls

When Claude invokes a tool from an SDK MCP server, the CLI sends an
`mcp_message` request:

```json
{
  "type": "sdk_control_request",
  "request": {
    "subtype": "mcp_message",
    "request_id": "mcp_1",
    "server_name": "calculator",
    "message": {
      "jsonrpc": "2.0",
      "id": "msg_1",
      "method": "tools/call",
      "params": {
        "name": "add",
        "arguments": {"a": 7, "b": 4}
      }
    }
  }
}
```

The SDK invokes the tool handler and responds. The response must wrap the
JSONRPC result in an `mcp_response` field:

```json
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "mcp_1",
    "response": {
      "mcp_response": {
        "jsonrpc": "2.0",
        "id": "msg_1",
        "result": {
          "content": [{"type": "text", "text": "11"}],
          "isError": false
        }
      }
    }
  }
}
```

The `mcp_response` wrapper is required. Without it, the CLI will not receive
the response correctly and will timeout after 60 seconds waiting.

### MCP Protocol Handshake

Before calling tools, the CLI performs an MCP protocol handshake with each SDK
server. It sends an `initialize` method:

```json
{
  "type": "sdk_control_request",
  "request": {
    "subtype": "mcp_message",
    "request_id": "mcp_0",
    "server_name": "calculator",
    "message": {
      "jsonrpc": "2.0",
      "id": "init_1",
      "method": "initialize",
      "params": {}
    }
  }
}
```

The SDK responds with server capabilities:

```json
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "mcp_0",
    "response": {
      "mcp_response": {
        "jsonrpc": "2.0",
        "id": "init_1",
        "result": {
          "protocolVersion": "2025-11-25",
          "capabilities": {
            "tools": {"listChanged": false}
          },
          "serverInfo": {
            "name": "calculator",
            "version": "1.0.0"
          }
        }
      }
    }
  }
}
```

The CLI then sends a `notifications/initialized` notification (no response
required, but the SDK should acknowledge it) and a `tools/list` request to
enumerate available tools.

## Environment Variables

The CLI respects several environment variables:

**ANTHROPIC_API_KEY** - API key for authentication. Required unless using OAuth.

**CLAUDE_CODE_OAUTH_TOKEN** - OAuth token for Max plan users.

**CLAUDE_CONFIG_DIR** - Override the config directory (default: `~/.claude`).

**CLAUDE_CODE_ENTRYPOINT** - Set by SDKs to identify the entry point (e.g., `sdk-go`).

**CLAUDE_AGENT_SDK_VERSION** - SDK version for debugging and analytics.

## Error Handling

Protocol errors are returned as control responses with `subtype: "error"`:

```json
{
  "type": "control_response",
  "response": {
    "subtype": "error",
    "request_id": "req_1",
    "error": "invalid request format"
  }
}
```

Tool errors should be returned as successful responses with error content, not
protocol errors:

```json
{
  "type": "control_response",
  "response": {
    "subtype": "success",
    "request_id": "mcp_1",
    "response": {
      "mcp_response": {
        "jsonrpc": "2.0",
        "id": "msg_1",
        "result": {
          "content": [{"type": "text", "text": "Division by zero"}],
          "isError": true
        }
      }
    }
  }
}
```

## Implementation Notes

A few lessons learned during SDK development:

The `mcp_response` wrapper requirement is not documented anywhere. The
TypeScript SDK wraps responses this way (see `sdk.mjs` line 13689: `return {
mcp_response: response }`), but this isn't mentioned in any official
documentation.

Request IDs must be unique within a session. The SDK should generate IDs that
won't collide with CLI-generated IDs.

The CLI buffers stdin, so writes should be followed by a newline and flush.
Each JSON message must be on a single line.

Graceful shutdown is achieved by closing stdin. The CLI will finish any pending
work and exit. If it doesn't exit within a reasonable timeout, sending SIGTERM
is appropriate.
