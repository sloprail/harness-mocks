package replay

import (
	"encoding/json"
	"fmt"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// mockCall is the mock's script name and input for a unified call; the input is
// copied, as the spawn's script parameter is added to it.
func mockCall(c core.Call) scriptCall {
	in := make(map[string]any, len(c.Input))
	for k, v := range c.Input {
		in[k] = v
	}
	hookID := in[hookIDKey] // the id the call's hooks name it by, when it is not the call's own
	delete(in, hookIDKey)
	name := "Shell"
	switch c.Tool {
	case core.ToolSpawn:
		name = "Task"
		delete(in, "message")
	case core.ToolReadFile:
		name = "Read"
		in["file_path"] = in["path"]
		delete(in, "path")
	case core.ToolWriteFile:
		name = "Write"
		in["file_path"] = in["path"]
		delete(in, "path")
	case core.ToolEditFile:
		name = "Edit"
		in["file_path"] = in["path"]
		delete(in, "path")
	case core.ToolSearchTools:
		name = "GetDynamicTools"
	case core.ToolAwaitTask:
		name = "AwaitShell"
		if id, ok := in["task"]; ok {
			in["shell_id"] = id
			delete(in, "task")
		}
	case core.ToolSearchFiles:
		name = "Grep"
	case core.ToolDeleteFile:
		name = "Delete"
		in["file_path"] = in["path"]
		delete(in, "path")
	case core.ToolMCP:
		name = fmt.Sprintf("mcp__%v__%v", in["server"], in["tool"])
		description := in["description"]
		in, _ = in["arguments"].(map[string]any)
		if in == nil {
			in = map[string]any{}
		}
		if description != nil { // the mock takes the model's description of the call in a key of its own
			in["__description"] = description
		}
	}
	if hookID != nil && c.Tool != core.ToolMCP { // an MCP call's hooks always name it by an id of their own
		in["hook_tool_use_id"] = hookID
	}
	return scriptCall{Name: name, Input: in}
}

// script is the mock script that plays the steps. The mock runs it once per
// turn and the session file holds the agent's records so far, so the step to
// play is the one that starts where the file stands: each step adds as many
// assistant records as its lines say. Past the last step it prints nothing,
// which ends the run. Call ids are unique across the run's scripts (tag), as
// the real ones are.
func script(tag string, steps []step, after int) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nn=$(grep -c '\"role\":\"assistant\"' \"$A10N_MOCK_SESSION_FILE\" 2>/dev/null)\ncase ${n:-0} in\n")
	at := after
	for i, s := range steps {
		fmt.Fprintf(&b, "%d) printf '%%s\\n' '%s' ;;\n", at, shellQuote(stepLine(fmt.Sprintf("toolu_%s_%d", tag, i), s)))
		at += s.lines()
	}
	b.WriteString("esac\n")
	return b.String()
}

// stepLine is the assistant line the script prints for a step: what the model
// thought, the text it said, then its calls, as the blocks of one record.
func stepLine(id string, s step) string {
	var blocks []any
	if s.thought != nil {
		block := map[string]any{"type": "thinking", "thinking": s.thought.Text}
		for k, v := range s.thought.Fields {
			block[k] = v
		}
		blocks = append(blocks, block)
	}
	if s.said != nil {
		blocks = append(blocks, map[string]any{"type": "text", "text": *s.said})
	}
	for j, c := range s.calls {
		blocks = append(blocks, map[string]any{"type": "tool_use", "id": fmt.Sprintf("%s_%d", id, j), "name": c.Name, "input": c.Input})
	}
	b, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{"role": "assistant", "content": blocks}})
	return string(b)
}

// shellQuote is s as the inside of a single-quoted shell word.
func shellQuote(s string) string { return strings.ReplaceAll(s, "'", `'\''`) }
