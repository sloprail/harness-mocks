package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fetch runs a script that calls WebFetch once with the given input, with a hook logging the
// PostToolUse and PostToolUseFailure payloads, and returns the run's exit code, output and the hook log.
func fetch(t *testing.T, input string) (code int, out, hookLog string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "hooks.log")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >> "+log+"\necho >> "+log+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}],"PostToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}],"PostToolUseFailure":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]}}`), 0o644))
	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"wf1","name":"WebFetch","input":`+input+`}]}}'
`), 0o755))
	out, code = runInDir(t, dir, nil, "--script", script, "--session-id", "fetch-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, ".cfg"), "-p", "go")
	data, _ := os.ReadFile(log)
	return code, out, string(data)
}

// TestT021_01_AFetchIsAnsweredWithTheScriptedResult: the answer the script gives the call is the text the
// agent gets and the result's own, with the size and the status of the page and the url, as the real tool
// shapes it; PostToolUse carries that result (recorded: runs/web-fetch-tool).
// sr:proves web-fetch-tool/claude
func TestT021_01_AFetchIsAnsweredWithTheScriptedResult(t *testing.T) {
	code, out, hooks := fetch(t, `{"url":"https://example.com","prompt":"Heading?","mock_result":{"result":"The heading is Example Domain.","bytes":577}}`)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"content":"The heading is Example Domain."`)
	assert.Contains(t, out, `"tool_use_result":{"bytes":577,"code":200,"codeText":"OK","durationMs":0,"result":"The heading is Example Domain.","url":"https://example.com"}`)
	assert.NotContains(t, out, `"is_error":true`)
	assert.Contains(t, hooks, `"tool_response":{"bytes":577,"code":200,"codeText":"OK","durationMs":0,"result":"The heading is Example Domain.","url":"https://example.com"}`)
	assert.Equal(t, 2, strings.Count(hooks, `"hook_event_name"`), "PreToolUse and PostToolUse")
	assert.NotContains(t, hooks, "mock_result", "the hooks are told the real tool's input")
	assert.NotContains(t, out, "mock_result", "nor does a frame of the call show it")
}

// TestT021_02_ALocalAddressIsRefusedBeforeAnyRequest: localhost and a host name without a dot are an error
// result pointing the agent at curl in the shell, which fails the call (PostToolUseFailure), with the
// script's answer, if it gave one, never used (recorded: runs/web-fetch-tool).
// sr:proves web-fetch-tool/claude
func TestT021_02_ALocalAddressIsRefusedBeforeAnyRequest(t *testing.T) {
	for _, url := range []string{"http://localhost:1/", "https://intranet/wiki", "http://LocalHost/x"} {
		code, out, hooks := fetch(t, `{"url":"`+url+`","prompt":"Summarize.","mock_result":{"result":"never shown"}}`)
		require.Equal(t, 0, code, out)
		assert.Contains(t, out, `"is_error":true`, url)
		assert.Contains(t, out, `"content":"WebFetch cannot fetch localhost or other hostnames without a dot. To reach a local server, use Bash with curl instead."`, url)
		assert.Contains(t, out, `"tool_use_result":"Error: WebFetch cannot fetch localhost or other hostnames without a dot.`, url)
		assert.NotContains(t, out, `"content":"never shown"`, url)
		assert.Contains(t, hooks, `"hook_event_name":"PostToolUseFailure"`, url)
	}
}

// TestT021_03_AFetchTheMockCannotAnswerIsAnError: a fetch the script gives no result is an error result
// saying so, and a url that is no http address fails the run (adr/tool-calls-validated): the mock reaches
// no web.
func TestT021_03_AFetchTheMockCannotAnswerIsAnError(t *testing.T) {
	code, out, _ := fetch(t, `{"url":"https://example.com","prompt":"Heading?"}`)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, "the mock fetches no page")
	code, out, _ = fetch(t, `{"url":"ftp://example.com/x","prompt":"x","mock_result":{}}`)
	require.NotZero(t, code, out)
	assert.Contains(t, out, "url")
}
