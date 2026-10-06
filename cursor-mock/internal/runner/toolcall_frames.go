package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/toolcall"
)

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func jsonLine(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

// toolFrame is a tool_call frame: the call's kind and args, and, once it has
// ended, its result.
func toolFrame(session, id, subtype string, c toolexec.Call, result map[string]any, contexts []any) []byte {
	args := c.Args
	switch c.Kind {
	case "shellToolCall":
		args = c.ShellFrameArgs(id, session, c.Request)
	case "grepToolCall", "deleteToolCall", "mcpToolCall", "getMcpToolsToolCall":
		// these tools' args name their own call (recorded: runs/hook-matchers-grep-delete,
		// runs/hook-matchers-mcp)
		args = map[string]any{"toolCallId": id}
		for k, v := range c.Args {
			args[k] = v
		}
	}
	body := map[string]any{"args": args}
	if _, failed := result["error"]; failed && (c.Kind == "readToolCall" || c.Kind == "editToolCall") {
		body = map[string]any{} // a file call that failed or was refused is reported by its result alone (recorded: runs/tool-failure, runs/pretool-refusal-file-tools, runs/before-read-refusal)
	} else if _, rejected := result["rejected"]; rejected && c.Kind == "shellToolCall" {
		body = map[string]any{} // a command a hook refused is reported by its result alone (recorded: runs/pretool-refusal)
	} else if c.Kind == "shellToolCall" && c.Described != "" { // the model's description sits beside the args too
		body["description"] = c.Described
	}
	if d, ok := args["__description"]; ok { // an MCP call's description sits beside its args (recorded: runs/hook-matchers-mcp)
		body["description"] = d
		args = copyWithout(args, "__description")
		body["args"] = args
	}
	if result != nil {
		body["result"] = result
	}
	return jsonLine(map[string]any{
		"type": "tool_call", "subtype": subtype, "call_id": id, "session_id": session,
		"tool_call": envelope(map[string]any{c.Kind: body}, id, contexts),
	})
}

// envelope is what every tool call frame carries beside its call: the call's id
// again and the context the hooks gave the agent for the call (recorded: every
// tool_call frame of runs/*, the contexts in runs/additional-context).
func envelope(call map[string]any, id string, contexts []any) map[string]any {
	if contexts == nil {
		contexts = []any{}
	}
	call["toolCallId"], call["hookAdditionalContexts"] = id, contexts
	return call
}

// startedFrame opens a tool call on the stream.
func startedFrame(session, id string, c toolexec.Call) []byte {
	return toolFrame(session, id, "started", c, nil, nil)
}

// completedFrame ends a tool call that ran.
func completedFrame(session, id string, c toolexec.Call, result map[string]any, contexts []any) []byte {
	return toolFrame(session, id, "completed", c, result, contexts)
}

// rejectedFrame ends a call a hook refused, with the reason the agent was
// given. A shell command's result is a rejection; a file tool's is an error
// carrying the reason, shaped as that tool's other errors are (recorded:
// runs/pretool-refusal for a command, runs/pretool-refusal-file-tools for a
// Write and a Read).
func rejectedFrame(session, id string, c toolexec.Call, reason string, contexts []any) []byte {
	switch c.Kind {
	case "editToolCall":
		return toolFrame(session, id, "completed", c, map[string]any{
			"error": map[string]any{"path": "", "error": reason, "modelVisibleError": reason}}, contexts)
	case "readToolCall":
		return errorFrame(session, id, c, reason, contexts)
	}
	rejected := map[string]any{"reason": reason, "isReadonly": false}
	if c.Kind == "shellToolCall" {
		rejected["command"], rejected["workingDirectory"] = c.Command(), ""
	} else {
		rejected["path"] = c.Args["path"]
	}
	return toolFrame(session, id, "completed", c, map[string]any{"rejected": rejected}, contexts)
}

// invalidFrame ends a call whose input lacks required parameters. A Task call
// is worded as Cursor words it (recorded: runs/agent-input-validation); the
// mock words the other tools' calls itself.
func invalidFrame(session string, c toolcall.Call, missing []string) []byte {
	call := toolexec.FromScript(c.Name, c.Input)
	if call.Kind == "taskToolCall" {
		return toolFrame(session, c.ID, "completed", call,
			map[string]any{"error": map[string]any{"error": toolexec.InvalidArguments(missing)}}, nil)
	}
	return errorFrame(session, c.ID, call,
		fmt.Sprintf("%s: missing required parameter(s): %s", c.Name, strings.Join(missing, ", ")), nil)
}

// errorFrame ends a call that could not run.
func errorFrame(session, id string, c toolexec.Call, message string, contexts []any) []byte {
	return toolFrame(session, id, "completed", c, map[string]any{"error": map[string]any{"errorMessage": message}}, contexts)
}

func copyWithout(m map[string]any, key string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}
