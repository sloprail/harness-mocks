package replay

// More of the unified tool vocabulary (see replay.go): the tools a harness's
// mock models besides the shell, files and sub-agents.
const (
	// ToolSearchFiles searches the workspace's files: Input "pattern" (string).
	ToolSearchFiles = "search_files"
	// ToolDeleteFile deletes a file: Input "path" (string).
	ToolDeleteFile = "delete_file"
	// ToolMCP calls a tool of an MCP server: Input "server" and "tool" (strings)
	// and "arguments" (an object).
	ToolMCP = "mcp_call"
)

// Thinking is what the model thought in one response, when the recording holds it:
// the text, and what the harness says of the model that thought (opaque to the
// core). It is put on the first call of the response, or on the answer.
type Thinking struct {
	Text   string
	Fields map[string]any
}
