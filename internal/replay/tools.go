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
