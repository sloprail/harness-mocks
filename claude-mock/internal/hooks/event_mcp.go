package hooks

import (
	"encoding/json"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// withMCPServer is the payload of a tool event with the server of an MCP tool named after the tool:
// mcp_server {name, source}, written last (recorded: snapshots/runs/mcp-tool, where the server was one
// of the run's --mcp-config, source "dynamic").
// sr:docs https://code.claude.com/docs/en/hooks#pretooluse-input
func withMCPServer(b []byte, toolName string) []byte {
	server, _, ok := tools.MCPName(toolName)
	if !ok || len(b) < 2 {
		return b
	}
	extra, err := json.Marshal(map[string]string{"name": server, "source": "dynamic"})
	if err != nil {
		return b
	}
	return append(b[:len(b)-1:len(b)-1], append([]byte(`,"mcp_server":`), append(extra, '}')...)...)
}
