package tools

import "strings"

// MCPName splits the name of a tool of an MCP server, mcp__<server>__<tool> (recorded:
// snapshots/runs/mcp-tool, where the server "browser" gave mcp__browser__fill_form), into
// the server and the tool; ok is false for any other name, or one with either part empty.
//
// sr:capability mcp-tool
func MCPName(name string) (server, tool string, ok bool) {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok {
		return "", "", false
	}
	server, tool, ok = strings.Cut(rest, "__")
	return server, tool, ok && server != "" && tool != ""
}

// IsMCPName reports whether name is mcp__<server>__<tool>.
func IsMCPName(name string) bool { _, _, ok := MCPName(name); return ok }

// MCPText is the text of an MCP tool's result: its text blocks, one per line. A failed call is given
// to the agent as this text alone (recorded: the error of runs/mcp-tool's refused download).
func MCPText(content []map[string]any) string {
	var lines []string
	for _, b := range content {
		if t, _ := b["text"].(string); b["type"] == "text" {
			lines = append(lines, t)
		}
	}
	return strings.Join(lines, "\n")
}

// MCPDisplayName is how the harness titles an MCP tool in the stream: its name's words, capitalised
// (recorded: fill_form is "Fill Form").
func MCPDisplayName(tool string) string {
	words := strings.FieldsFunc(tool, func(r rune) bool { return r == '_' || r == '-' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
