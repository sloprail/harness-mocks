package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callMCP runs a script that calls the tool name once with the given input, with a hook logging every
// tool event's payload, and returns the run's exit code, output and the hook log. extra are further
// arguments of the run.
func callMCP(t *testing.T, name, input string, extra ...string) (code int, out, hookLog string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "hooks.log")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >> "+log+"\necho >> "+log+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	events := ""
	for _, e := range []string{"PreToolUse", "PostToolUse", "PostToolUseFailure"} {
		events += `,"` + e + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]`
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(`{"hooks":{`+events[1:]+`}}`), 0o644))
	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"mcp1","name":"`+name+`","input":`+input+`}]}}'
`), 0o755))
	args := append([]string{"--script", script, "--session-id", "mcp-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, ".cfg")}, extra...)
	out, code = runInDir(t, dir, nil, append(args, "-p", "go")...)
	data, _ := os.ReadFile(log)
	return code, out, string(data)
}

const fillArgs = `"fields":[{"name":"email","value":"x"}]`

// TestT024_01_AToolResultIsTheServersTextBlocks: the blocks the script gives the call are the tool_result's
// content and its structured result alike, with no is_error; the stream's frame of the call titles the
// tool and names the server; the hooks see the call under mcp__<server>__<tool> with the tool's input
// alone (the script's own result is not among it), the server it comes from, and the blocks as
// PostToolUse's tool_response (recorded: runs/mcp-tool).
// sr:proves mcp-tool/claude
func TestT024_01_AToolResultIsTheServersTextBlocks(t *testing.T) {
	code, out, hooks := callMCP(t, "mcp__browser__fill_form", `{`+fillArgs+`,"mock_result":{"content":[{"type":"text","text":"Filled 1 fields"}]}}`)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"content":[{"text":"Filled 1 fields","type":"text"}]`)
	assert.Contains(t, out, `"tool_use_result":[{"text":"Filled 1 fields","type":"text"}]`)
	assert.NotContains(t, out, `"is_error":true`)
	assert.Contains(t, out, `"tool_use_meta":[{"display_name":"Fill Form","id":"mcp1","server_display_name":"browser"}]`)
	assert.Contains(t, hooks, `"hook_event_name":"PreToolUse","tool_name":"mcp__browser__fill_form","tool_input":{`+fillArgs+`}`)
	assert.Contains(t, hooks, `"tool_response":[{"text":"Filled 1 fields","type":"text"}]`)
	assert.Equal(t, 2, strings.Count(hooks, `"mcp_server":{"name":"browser","source":"dynamic"}`), "PreToolUse and PostToolUse")
	assert.NotContains(t, hooks, "mock_result")
}

// TestT024_02_AnErrorResultIsItsTextAndFailsTheCall: a result the server marks isError is given to the agent
// as its text alone, as an error, with the structured result "Error: <text>", and PostToolUseFailure fires
// with that text and the server (recorded: runs/mcp-tool, the refused download).
// sr:proves mcp-tool/claude
// sr:proves tool-failure-hook/claude
func TestT024_02_AnErrorResultIsItsTextAndFailsTheCall(t *testing.T) {
	code, out, hooks := callMCP(t, "mcp__browser__download_file",
		`{"url":"http://example.com/f","mock_result":{"isError":true,"content":[{"type":"text","text":"only https is allowed"}]}}`)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"content":"only https is allowed","is_error":true`)
	assert.Contains(t, out, `"tool_use_result":"Error: only https is allowed"`)
	for _, part := range []string{`"hook_event_name":"PostToolUseFailure"`, `"tool_name":"mcp__browser__download_file"`, `"tool_input":{"url":"http://example.com/f"}`, `"error":"only https is allowed"`} {
		assert.Contains(t, hooks, part)
	}
	assert.Contains(t, hooks, `"mcp_server":{"name":"browser","source":"dynamic"}`)
	assert.NotContains(t, hooks, `"hook_event_name":"PostToolUse"`, "no PostToolUse")
}

// TestT024_03_ANameIsServerAndTool: any server and tool of the form is called, a plugin's scoped server
// segment and a hyphenated server name among them, whatever the run's --tools say of the built-in tools;
// a name that is not the form is an unknown tool and fails the run (adr/tool-calls-validated).
// sr:proves mcp-tool/claude
func TestT024_03_ANameIsServerAndTool(t *testing.T) {
	ok := `{"mock_result":{"content":[{"type":"text","text":"ok"}]}}`
	for _, name := range []string{"mcp__plugin_x_browser__browser_fill_form", "mcp__brave-search__web", "mcp__a__b__c"} {
		code, out, _ := callMCP(t, name, ok, "--tools=Bash")
		require.Equal(t, 0, code, name+": "+out)
		assert.Contains(t, out, `"content":[{"text":"ok","type":"text"}]`, name)
	}
	for _, name := range []string{"mcp__browser", "mcp____tool", "mcp__server__", "mcp_server_tool"} {
		code, out, _ := callMCP(t, name, ok)
		require.NotZero(t, code, name+": "+out)
		assert.Contains(t, out, "unknown tool", name)
	}
}

// TestT024_04_ACallTheMockCannotAnswerIsAnError: a call the script gives no result is an error result saying
// so: the mock starts no server.
func TestT024_04_ACallTheMockCannotAnswerIsAnError(t *testing.T) {
	code, out, _ := callMCP(t, "mcp__browser__screenshot", `{}`)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, "the mock runs no MCP server")
}
