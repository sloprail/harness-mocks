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
	// Replace is a StrReplace's old and new text: the call edits the file by it
	// and the hooks see the whole file it makes (recorded: runs/file-tools).
	Replace *[2]string
	// Unmodeled are the keys of a script's input the mock does not take for the
	// tool (a Grep's path, glob and the rest): the call fails rather than ignore them.
	Unmodeled []string
	// BlockMs is the shell call's block_until_ms, when it gave one, and Described
	// its description: what the frames show of it (shellargs.go).
	BlockMs   *int
	Described string
	// HookID is the id the call's hooks name it by when it is not the call's own (a
	// mock-only input of the script, hook_tool_use_id): Cursor's hooks were recorded
	// naming a call by an id of their own in some runs and by the call's id in others.
	HookID string
	// ShellID is the id a background shell is to have (a mock-only input of the script,
	// task_id): the harness numbers its shells itself, and a later wait names the one it means.
	ShellID string
	// Request is the id of the model request the call was made in (the frames' requestId).
	Request string
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
		// a command left in the background has no timeout; any other has the one
		// the model gave (block_until_ms), or the default (recorded:
		// runs/task-notifications-inturn, runs/task-notifications-bg)
		in := map[string]any{"command": c.Command(), "cwd": c.str("workingDirectory")}
		if !c.Background() {
			in["timeout"] = 30000
			if c.BlockMs != nil {
				in["timeout"] = *c.BlockMs
			}
		}
		return in
	case "readToolCall":
		return map[string]any{"file_path": c.Path(dir)}
	case "taskToolCall":
		return map[string]any{"description": c.str("description"), "prompt": c.str("prompt"), "subagent_type": "generalPurpose"}
	case "grepToolCall":
		return map[string]any{"pattern": c.str("pattern")}
	case "deleteToolCall":
		return map[string]any{"file_path": c.Path(dir)}
	case "mcpToolCall":
		return c.Args["args"].(map[string]any)
	default:
		content := c.str("streamContent")
		if c.Replace != nil {
			content, _ = replaced(c, dir)
		}
		return map[string]any{"file_path": c.Path(dir), "content": content}
	}
}

// TranscriptInput is the call's input as Cursor writes it into the conversation's
// transcript, which is not the frame's args: a file's path is the absolute one, a
// write's text is "contents" (the frame's streamContent), and a StrReplace keeps
// its old_string and new_string (recorded: runs/file-tools, runs/tool-failure,
// runs/pretool-refusal-file-tools).
func (c Call) TranscriptInput(dir string) map[string]any {
	switch c.Kind {
	case "editToolCall":
		if c.Replace != nil {
			return map[string]any{"path": c.Path(dir), "old_string": c.Replace[0], "new_string": c.Replace[1]}
		}
		return map[string]any{"path": c.Path(dir), "contents": c.str("streamContent")}
	case "readToolCall", "deleteToolCall":
		in := map[string]any{}
		for k, v := range c.Args {
			in[k] = v
		}
		in["path"] = c.Path(dir)
		return in
	}
	return c.Args
}

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
		return ran(func() Result { return mcp(ctx, c, dir, env) })
	}
	return timed(c, dir)
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }
