package toolexec

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
