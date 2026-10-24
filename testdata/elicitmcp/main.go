// Command elicitmcp is a stdio MCP server for the integration tests. Its one
// tool, signup, asks the client for a form through elicitation/create before
// it answers, so a test can observe the elicitation the CLI forwards.
//
// The form's properties are written in non-alphabetical order on purpose: a
// client that reports their declared order cannot be recovering it by
// sorting the keys.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

const formSchema = `{"type":"object","properties":{` +
	`"zeta":{"type":"string"},` +
	`"alpha":{"type":"string"},` +
	`"mid":{"type":"string"}}}`

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<24)
	out := json.NewEncoder(os.Stdout)

	reply := func(id json.RawMessage, result string) {
		_ = out.Encode(message{
			JSONRPC: "2.0", ID: id, Result: json.RawMessage(result),
		})
	}

	for in.Scan() {
		var msg message
		if err := json.Unmarshal(in.Bytes(), &msg); err != nil {
			continue
		}

		switch msg.Method {
		case "initialize":
			var params struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(msg.Params, &params)
			reply(msg.ID, fmt.Sprintf(`{"protocolVersion":%q,`+
				`"capabilities":{"tools":{}},`+
				`"serverInfo":{"name":"elicitmcp","version":"0.0.1"}}`,
				params.ProtocolVersion))

		case "tools/list":
			reply(msg.ID, `{"tools":[{"name":"signup",`+
				`"description":"Sign the user up. Asks them for a form.",`+
				`"inputSchema":{"type":"object"}}]}`)

		case "tools/call":
			_ = out.Encode(message{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`"elicit-1"`),
				Method:  "elicitation/create",
				Params: json.RawMessage(`{"message":"Fill in the form",` +
					`"requestedSchema":` + formSchema + `}`),
			})

			// Wait for the client's answer before finishing the call.
			action := "none"
			for in.Scan() {
				var resp message
				if json.Unmarshal(in.Bytes(), &resp) != nil {
					continue
				}
				if string(resp.ID) != `"elicit-1"` || resp.Method != "" {
					continue
				}
				var res struct {
					Action string `json:"action"`
				}
				_ = json.Unmarshal(resp.Result, &res)
				action = res.Action
				break
			}
			reply(msg.ID, fmt.Sprintf(
				`{"content":[{"type":"text","text":"elicitation %s"}]}`,
				action))

		default:
			// Notifications need no answer; anything else gets an empty
			// result so the client never waits on us.
			if len(msg.ID) > 0 && msg.Method != "" {
				reply(msg.ID, `{}`)
			}
		}
	}
}
