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
func toolFrame(session, id, subtype string, c toolexec.Call, result map[string]any) []byte {
	body := map[string]any{"args": c.Args}
	if result != nil {
		body["result"] = result
	}
	return jsonLine(map[string]any{
		"type": "tool_call", "subtype": subtype, "call_id": id, "session_id": session,
		"tool_call": map[string]any{c.Kind: body},
	})
}

// startedFrame opens a tool call on the stream.
func startedFrame(session, id string, c toolexec.Call) []byte {
	return toolFrame(session, id, "started", c, nil)
}

// completedFrame ends a tool call that ran.
func completedFrame(session, id string, c toolexec.Call, result map[string]any) []byte {
	return toolFrame(session, id, "completed", c, result)
}

// rejectedFrame ends a call a hook refused, with the reason the agent was
// given. A shell command's result is a rejection; a file tool's is an error
// carrying the reason, shaped as that tool's other errors are (recorded:
// runs/pretool-refusal for a command, runs/pretool-refusal-file-tools for a
// Write and a Read).
func rejectedFrame(session, id string, c toolexec.Call, reason string) []byte {
	switch c.Kind {
	case "editToolCall":
		return toolFrame(session, id, "completed", c, map[string]any{
			"error": map[string]any{"path": "", "error": reason, "modelVisibleError": reason}})
	case "readToolCall":
		return errorFrame(session, id, c, reason)
	}
	rejected := map[string]any{"reason": reason, "isReadonly": false}
	if c.Kind == "shellToolCall" {
		rejected["command"], rejected["workingDirectory"] = c.Command(), ""
	} else {
		rejected["path"] = c.Args["path"]
	}
	return toolFrame(session, id, "completed", c, map[string]any{"rejected": rejected})
}

// invalidFrame ends a call whose input lacks required parameters. A Task call
// is worded as Cursor words it (recorded: runs/agent-input-validation); the
// mock words the other tools' calls itself.
func invalidFrame(session string, c toolcall.Call, missing []string) []byte {
	call := toolexec.FromScript(c.Name, c.Input)
	if call.Kind == "taskToolCall" {
		return toolFrame(session, c.ID, "completed", call,
			map[string]any{"error": map[string]any{"error": toolexec.InvalidArguments(missing)}})
	}
	return errorFrame(session, c.ID, call,
		fmt.Sprintf("%s: missing required parameter(s): %s", c.Name, strings.Join(missing, ", ")))
}

// errorFrame ends a call that could not run.
func errorFrame(session, id string, c toolexec.Call, message string) []byte {
	return toolFrame(session, id, "completed", c, map[string]any{"error": map[string]any{"errorMessage": message}})
}
