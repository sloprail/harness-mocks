package toolexec

import (
	"encoding/json"
	"sort"
)

// FromScript is the Cursor call a scenario script's tool call stands for.
func FromScript(name string, input json.RawMessage) Call {
	var in map[string]any
	_ = json.Unmarshal(input, &in)
	str := func(k string) string { s, _ := in[k].(string); return s }
	kind, _, _, _ := lookup(name)
	c := Call{Kind: kind, Args: map[string]any{}, HookID: str("hook_tool_use_id")}
	switch c.Kind {
	case "shellToolCall":
		c.Args["command"] = str("command")
		c.Described, c.ShellID = str("description"), str("task_id")
		if v, ok := in["block_until_ms"].(float64); ok && v > 0 {
			ms := int(v)
			c.BlockMs = &ms
		}
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
		if v, ok := in["limit"].(float64); ok {
			c.Args["limit"] = int(v)
		}
	case "editToolCall":
		c.Args["path"], c.Args["streamContent"] = str("file_path"), str("content")
		if name == "Edit" { // a StrReplace: its stream content is the new text only
			c.Args["streamContent"] = str("new_string")
			c.Replace = &[2]string{str("old_string"), str("new_string")}
		}
	case "grepToolCall":
		c.Args["pattern"], c.Args["caseInsensitive"], c.Args["multiline"], c.Args["offset"] = str("pattern"), false, false, 0
		for k := range in {
			if k != "pattern" && k != "hook_tool_use_id" {
				c.Unmodeled = append(c.Unmodeled, k)
			}
		}
		sort.Strings(c.Unmodeled)
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
