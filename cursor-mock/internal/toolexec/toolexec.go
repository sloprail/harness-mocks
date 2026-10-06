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

// FromScript is the Cursor call a scenario script's tool call stands for.
func FromScript(name string, input json.RawMessage) Call {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	str := func(k string) string { s, _ := in[k].(string); return s }
	kind, _, _, _ := lookup(name)
	c := Call{Kind: kind, Args: map[string]any{}}
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
		if name == "Edit" { // a StrReplace: its stream content is the new text only
			c.Args["streamContent"] = str("new_string")
			c.Replace = &[2]string{str("old_string"), str("new_string")}
		}
	case "grepToolCall":
		c.Args["pattern"], c.Args["caseInsensitive"], c.Args["multiline"], c.Args["offset"] = str("pattern"), false, false, 0
	case "deleteToolCall":
		c.Args["path"] = str("file_path")
	case "mcpToolCall":
		server, tool, _ := mcpName(name)
		args := in
		if args == nil {
			args = map[string]any{}
		}
		if d, ok := args["__description"]; ok { // the model's description of the call: the frame carries it beside the args
			c.Args["__description"] = d
			delete(args, "__description")
		}
		c.Args["name"], c.Args["args"], c.Args["providerIdentifier"], c.Args["toolName"] = server+"-"+tool, args, server, tool
		c.Args["smartModeApprovalOnly"], c.Args["skipApproval"], c.Args["serverIdentifier"] = false, false, server
	case "taskToolCall":
		c.Args["description"], c.Args["prompt"] = str("description"), str("prompt")
	}
	return c
}

// HookInput is the call's input as hooks see it.
func (c Call) HookInput(dir string) map[string]any {
	switch c.Kind {
	case "shellToolCall":
		return map[string]any{"command": c.Command(), "cwd": c.str("workingDirectory"), "timeout": 30000}
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
