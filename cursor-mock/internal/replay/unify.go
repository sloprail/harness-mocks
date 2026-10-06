package replay

import (
	"fmt"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// unify maps a tool_use block onto the unified vocabulary: Shell is a shell
// command, Task a spawn (its prompt is the message), Read and Write file reads
// and writes. Any other tool is not mapped: the mock has none of them.
func unify(block map[string]any) (core.Call, error) {
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]any)
	in := make(map[string]any, len(input)+1)
	for k, v := range input {
		in[k] = v
	}
	switch name {
	case "Grep":
		return core.Call{Tool: core.ToolSearchFiles, Input: in}, nil
	case "Delete":
		return core.Call{Tool: core.ToolDeleteFile, Input: in}, nil
	case "CallDynamicTool":
		args, _ := input["arguments"].(map[string]any)
		return core.Call{Tool: core.ToolMCP, Input: map[string]any{"server": input["namespace"], "tool": input["toolName"], "arguments": args}}, nil
	case "Shell":
		return core.Call{Tool: core.ToolShell, Input: in}, nil
	case "Task":
		prompt, _ := input["prompt"].(string)
		in["message"] = prompt
		return core.Call{Tool: core.ToolSpawn, Input: in}, nil
	case "Read":
		return core.Call{Tool: core.ToolReadFile, Input: in}, nil
	case "Write":
		in["content"] = in["contents"]
		delete(in, "contents")
		return core.Call{Tool: core.ToolWriteFile, Input: in}, nil
	}
	return core.Call{}, fmt.Errorf("the model called %s: the mock has no such tool", name)
}
