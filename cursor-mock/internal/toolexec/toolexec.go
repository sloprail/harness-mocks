// Package toolexec executes the Cursor tools the mock models (Shell, Read,
// Write): the results Cursor reports for them, and what its hooks are told.
package toolexec

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"
)

// Call is a tool call as Cursor names it: the kind of its tool_call frame
// (shellToolCall, readToolCall, editToolCall) and that frame's args.
type Call struct {
	Kind string
	Args map[string]any
}

// Result is what a tool call came to.
type Result struct {
	// Failed: the call ran and failed.
	Failed bool
	// Frame is the result object of the call's completed stream frame.
	Frame map[string]any
	// ToolOutput is the call's result as postToolUse reports it, a JSON string.
	ToolOutput string
	// ErrorMessage is what postToolUseFailure reports when the call failed.
	ErrorMessage string
	// Output is what a shell command printed, for afterShellExecution.
	Output string
	// Edits are the changes a file write made, for afterFileEdit.
	Edits []Edit
	Took  time.Duration
}

// Edit is one change a write made to a file.
type Edit struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
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
	case "readToolCall":
		c.Args["path"] = str("file_path")
	case "editToolCall":
		c.Args["path"], c.Args["streamContent"] = str("file_path"), str("content")
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

// Command is the shell line of a Shell call.
func (c Call) Command() string { return c.str("command") }

// HookInput is the call's input as hooks see it.
func (c Call) HookInput(dir string) map[string]any {
	switch c.Kind {
	case "shellToolCall":
		return map[string]any{"command": c.Command(), "cwd": c.str("workingDirectory"), "timeout": 30000}
	case "readToolCall":
		return map[string]any{"file_path": c.Path(dir)}
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
	}
	return write(c, dir)
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }
