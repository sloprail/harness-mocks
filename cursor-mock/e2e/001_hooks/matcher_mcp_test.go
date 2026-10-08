package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The recorded run runs/hook-matchers-mcp: a call of the echo tool of a local
// MCP server (the project's .cursor/mcp.json, started with --approve-mcps), with
// preToolUse hooks matched on MCP:echo, MCP:other and Shell and one with no
// matcher, and postToolUse hooks matched on MCP:echo and none.

// TestAMatcherIsTestedAgainstAnMCPToolsName: recorded, an MCP tool is named
// MCP:<tool> to hooks (here MCP:echo): the preToolUse and postToolUse hooks
// matched on MCP:echo run for the call, the ones matched on MCP:other and on
// Shell do not, and the hooks with no matcher do; the agent's reading of the
// tool's schema before the call is not a call hooks see. The mock calls the
// server and runs the same hooks, and the stream shows the same calls.
// sr:proves hook-matcher-filter/cursor
func TestAMatcherIsTestedAgainstAnMCPToolsName(t *testing.T) {
	got, want := replayWith(t, "hook-matchers-mcp", "--approve-mcps")
	conforms(t, got, want)

	for name, o := range map[string]observed{"recorded": want, "mock": got} {
		for _, r := range []string{
			"ran:pre-mcp-echo:preToolUse:MCP:echo::<nil>", "ran:pre-all:preToolUse:MCP:echo::<nil>",
			"ran:post-mcp-echo:postToolUse:MCP:echo::<nil>", "ran:post-all:postToolUse:MCP:echo::<nil>",
		} {
			require.Contains(t, o.results, r, name)
		}
		for _, r := range o.results {
			require.False(t, strings.Contains(r, "pre-mcp-other") || strings.Contains(r, "pre-Shell"), name+": "+r)
		}
	}
	var tools []string
	for _, f := range got.frames {
		if strings.Contains(f, "McpTool") || strings.Contains(f, "mcpToolCall") {
			tools = append(tools, f)
		}
	}
	require.Equal(t, []string{"tool_call/started/getMcpToolsToolCall/", "tool_call/completed/getMcpToolsToolCall/success", "tool_call/started/mcpToolCall/", "tool_call/completed/mcpToolCall/success"}, tools)
}

// TestAnMCPToolCallIsRefusedWithoutApproveMcps: MCP calls were recorded only
// with --approve-mcps; without it the mock answers the call with an error naming
// the flag rather than guess how Cursor asks.
func TestAnMCPToolCallIsRefusedWithoutApproveMcps(t *testing.T) {
	scratch := t.TempDir()
	script := filepath.Join(scratch, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
n=$(grep -c '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null)
if [ "${n:-0}" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"call_1","name":"mcp__local__echo","input":{"text":"HELLO"}}]}}'
else
  printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"DONE"}'
fi
`), 0o755))
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = t.TempDir(), []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "a refusal of something not modeled fails the run")
	require.Contains(t, string(out), "an MCP tool call is modeled only with --approve-mcps")
}
