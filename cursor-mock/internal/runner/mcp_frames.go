package runner

import (
	"context"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
	coresession "github.com/sloprail/harness-mocks/internal/session"
)

// readsMcpTool prints what precedes an MCP tool's call on the stream: the agent
// reading the tool's description and schema, a call of its own that starts and
// completes at once and that no hook sees (recorded: runs/hook-matchers-mcp).
func (s *session) readsMcpTool(ctx context.Context, tu scenario.ToolUse, c toolexec.Call) {
	id := coresession.NewID()
	g := toolexec.Call{Kind: "getMcpToolsToolCall", Args: map[string]any{"server": c.Args["providerIdentifier"], "toolName": c.Args["toolName"]}}
	s.forward(startedFrame(s.id, id, g))
	server, _ := c.Args["providerIdentifier"].(string)
	tool, _ := c.Args["toolName"].(string)
	content, err := toolexec.MCPDescribe(ctx, s.cfg.Dir, server, tool, s.cfg.Environ)
	if err != nil {
		s.forward(errorFrame(s.id, id, g, err.Error(), nil))
		return
	}
	s.forward(completedFrame(s.id, id, g, map[string]any{"success": map[string]any{"content": content}}, nil))
	s.named = true // the transcript exists by the time the call itself is made (recorded: runs/hook-matchers-mcp)
}
