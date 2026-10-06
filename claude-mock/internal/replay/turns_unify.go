package replay

import (
	"fmt"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// unify maps a tool_use block onto the unified vocabulary: Bash is a shell
// command, Agent a spawn (its prompt is the message). Any other tool is not
// mapped yet.
func unify(block map[string]any) (core.Call, error) {
	name, _ := block["name"].(string)
	input, _ := block["input"].(map[string]any)
	in := make(map[string]any, len(input)+1)
	for k, v := range input {
		in[k] = v
	}
	if tool, ok := fileTools[name]; ok {
		return core.Call{Tool: tool, Input: in}, nil
	}
	switch name {
	case "SendMessage":
		return core.Call{Tool: toolSend, Input: in}, nil
	case "Bash":
		return core.Call{Tool: core.ToolShell, Input: in}, nil
	case "Agent", "Task":
		prompt, _ := input["prompt"].(string)
		in["message"] = prompt
		return core.Call{Tool: core.ToolSpawn, Input: in}, nil
	}
	if name == "" {
		return core.Call{}, fmt.Errorf("a tool call with no name")
	}
	return core.Call{Tool: toolPrefix + name, Input: in}, nil // any other tool goes on under its own name: the mock runs or refuses it
}

// toolPrefix starts the unified name of a tool the claude adapter passes on as it is.
const toolPrefix = "claude:"
