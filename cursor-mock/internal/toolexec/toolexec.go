// Package toolexec executes the Cursor tools the mock models (Shell, Read,
// Write): the results Cursor reports for them, and what its hooks are told.
package toolexec

import (
	"context"
	"encoding/json"
	"path/filepath"
)

// Call is a tool call as Cursor names it: the kind of its tool_call frame
// (shellToolCall, readToolCall, editToolCall) and that frame's args.
type Call struct {
	Kind string
	Args map[string]any
}

// toolTable is the tools the mock models, by the name a scenario script gives
// them (the Claude Code names, with Cursor's Shell too): the kind of Cursor
// call, its name in hooks, and the parameters its input must carry.
var toolTable = map[string]struct {
	kind, hookName string
	required       []string
}{
	"Bash":  {"shellToolCall", "Shell", []string{"command"}},
	"Shell": {"shellToolCall", "Shell", []string{"command"}},
	"Read":  {"readToolCall", "Read", []string{"file_path"}},
	"Write": {"editToolCall", "Write", []string{"file_path", "content"}},
	// a sub-agent dispatch, whichever name it goes by (Task is Agent's old name)
	"Agent": {"taskToolCall", "Task", taskRequired},
	"Task":  {"taskToolCall", "Task", taskRequired},
}

// Required is the parameters a call to the named tool must carry, and whether
// the mock has the tool.
func Required(name string) ([]string, bool) {
	t, ok := toolTable[name]
	return t.required, ok
}

// FromScript is the Cursor call a scenario script's tool call stands for.
func FromScript(name string, input json.RawMessage) Call {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	str := func(k string) string { s, _ := in[k].(string); return s }
	c := Call{Kind: toolTable[name].kind, Args: map[string]any{}}
	switch c.Kind {
	case "shellToolCall":
		c.Args["command"] = str("command")
		// block_until_ms 0 (or run_in_background) is a shell left running in the
		// background (recorded: runs/task-notifications-bg).
		if v, ok := in["block_until_ms"].(float64); (ok && v == 0) || in["run_in_background"] == true {
			c.Args["isBackground"], c.Args["timeout"] = true, 0
			if d := str("description"); d != "" {
				c.Args["description"] = d
			}
		}
	case "readToolCall":
		c.Args["path"] = str("file_path")
	case "editToolCall":
		c.Args["path"], c.Args["streamContent"] = str("file_path"), str("content")
	case "taskToolCall":
		c.Args["description"], c.Args["prompt"] = str("description"), str("prompt")
	}
	return c
}

// Name is the tool's name in hooks: Shell, Read or Write.
func (c Call) Name() string {
	for _, t := range toolTable {
		if t.kind == c.Kind {
			return t.hookName
		}
	}
	return ""
}

func (c Call) str(key string) string { s, _ := c.Args[key].(string); return s }

// Path is the file a Read or Write call names, resolved against dir.
func (c Call) Path(dir string) string {
	p := c.str("path")
	if p != "" && !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p
}

// Background reports whether a Shell call asked to be left running.
func (c Call) Background() bool { b, _ := c.Args["isBackground"].(bool); return b }

// Description is what a Shell call says it does, if it says.
func (c Call) Description() string { return c.str("description") }

// Command is the shell line of a Shell call.
func (c Call) Command() string { return c.str("command") }

// HookInput is the call's input as hooks see it.
func (c Call) HookInput(dir string) map[string]any {
	switch c.Kind {
	case "shellToolCall":
		return map[string]any{"command": c.Command(), "cwd": c.str("workingDirectory"), "timeout": 30000}
	case "readToolCall":
		return map[string]any{"file_path": c.Path(dir)}
	case "taskToolCall":
		return map[string]any{"description": c.str("description"), "prompt": c.str("prompt"), "subagent_type": "generalPurpose"}
	default:
		return map[string]any{"file_path": c.Path(dir), "content": c.str("streamContent")}
	}
}

// Execute runs the call: a shell command in dir with env, or a file tool on
// the file it names.
func Execute(ctx context.Context, c Call, dir string, env []string) Result {
	switch c.Kind {
	case "shellToolCall":
		return shell(ctx, c, dir, env)
	case "readToolCall":
		return read(c, dir)
	case "taskToolCall":
		return task()
	}
	return write(c, dir)
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }
