package toolexec

import "encoding/json"

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
		return map[string]any{"file_path": c.Path(dir), "content": c.str("streamContent")}
	}
}
