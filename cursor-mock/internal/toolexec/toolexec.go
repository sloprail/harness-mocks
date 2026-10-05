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
	"Bash":   {"shellToolCall", "Shell", []string{"command"}},
	"Shell":  {"shellToolCall", "Shell", []string{"command"}},
	"Read":   {"readToolCall", "Read", []string{"file_path"}},
	"Write":  {"editToolCall", "Write", []string{"file_path", "content"}},
	"Grep":   {"grepToolCall", "Grep", []string{"pattern"}},
	"Delete": {"deleteToolCall", "Delete", []string{"file_path"}},
	// a sub-agent dispatch, whichever name it goes by (Task is Agent's old name)
	"Agent": {"taskToolCall", "Task", taskRequired},
	"Task":  {"taskToolCall", "Task", taskRequired},
}

// lookup is the table's entry for a tool name: an MCP tool's name is
// mcp__<server>__<tool>, a call the mock makes of the server's tool.
func lookup(name string) (kind, hookName string, required []string, ok bool) {
	if _, _, isMCP := mcpName(name); isMCP {
		return "mcpToolCall", "", nil, true
	}
	t, ok := toolTable[name]
	return t.kind, t.hookName, t.required, ok
}

// Required is the parameters a call to the named tool must carry, and whether
// the mock has the tool.
func Required(name string) ([]string, bool) {
	_, _, req, ok := lookup(name)
	return req, ok
}

// Name is the tool's name in hooks: Shell, Read or Write.
func (c Call) Name() string {
	if c.Kind == "mcpToolCall" {
		return "MCP:" + c.str("toolName")
	}
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

// Execute runs the call: a shell command in dir with env, or a file tool on
// the file it names.
func Execute(ctx context.Context, c Call, dir string, env []string) Result {
	switch c.Kind {
	case "shellToolCall":
		return shell(ctx, c, dir, env)
	case "taskToolCall":
		return task()
	case "grepToolCall":
		return ran(func() Result { return grep(c, dir) })
	case "deleteToolCall":
		return ran(func() Result { return deleteFile(c, dir) })
	case "mcpToolCall":
		return ran(func() Result { return mcp(ctx, c, dir) })
	}
	return timed(c, dir)
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }
