package replay

// More of the unified tool vocabulary (see replay.go): the tools a harness's
// mock models besides the shell, files and sub-agents.
const (
	// ToolSearchFiles searches the workspace's files: Input "pattern" (string).
	ToolSearchFiles = "search_files"
	// ToolDeleteFile deletes a file: Input "path" (string).
	ToolDeleteFile = "delete_file"
	// ToolEditFile replaces text in a file: Input "path", "old_string" and
	// "new_string" (strings).
	ToolEditFile = "edit_file"
	// ToolSearchTools searches the harness's own catalogue of tools: Input
	// "pattern" (string).
	ToolSearchTools = "search_tools"
	// ToolAwaitTask waits for a background task: Input "block_until_ms" (a number)
	// and, for a named task, "task" (string).
	ToolAwaitTask = "await_task"
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
