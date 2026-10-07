package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// search runs a script that calls WebSearch once with the given input, with a hook logging the PostToolUse
// payload, and returns the run's exit code, output and the hook log.
func search(t *testing.T, input string) (code int, out, hookLog string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "hooks.log")
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >> "+log+"\necho >> "+log+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}],"PostToolUse":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]}}`), 0o644))
	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"ws1","name":"WebSearch","input":`+input+`}]}}'
`), 0o755))
	out, code = runInDir(t, dir, nil, "--script", script, "--session-id", "search-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, ".cfg"), "-p", "go")
	data, _ := os.ReadFile(log)
	return code, out, string(data)
}

const found = `{"query":"iana example domains","mode":"standard","mock_result":{"links":[{"title":"Example Domains","url":"https://www.iana.org/help/example-domains"},{"title":".INT Policy & Procedures","url":"https://www.iana.org/domains/int/policy"}],"findings":"Based on the search results, example.com is reserved.","searchCount":1}}`

// TestT022_01_ASearchIsAnsweredWithTheScriptedResult: the text the agent gets is the query, the pages found as a
// JSON list of title and url (an "&" in a title kept as it is), the findings and the reminder to cite the
// sources; the result beside it holds the query, the pages under the id of the search with the findings, and
// the number of searches (recorded: runs/web-search-tool).
// sr:proves web-search-tool/claude
func TestT022_01_ASearchIsAnsweredWithTheScriptedResult(t *testing.T) {
	code, out, hooks := search(t, found)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"content":"Web search results for query: \"iana example domains\"\n\nLinks: [{\"title\":\"Example Domains\",\"url\":\"https://www.iana.org/help/example-domains\"},{\"title\":\".INT Policy & Procedures\",\"url\":\"https://www.iana.org/domains/int/policy\"}]\n\nBased on the search results, example.com is reserved.\n\n\nREMINDER: You MUST include the sources above in your response to the user using markdown hyperlinks."`)
	assert.Regexp(t, `"tool_use_result":\{"durationSeconds":0,"query":"iana example domains","results":\[\{"content":\[\{"title":"Example Domains","url":"https://www.iana.org/help/example-domains"\},\{"title":".INT Policy & Procedures","url":"https://www.iana.org/domains/int/policy"\}\],"tool_use_id":"srvtoolu_[0-9A-Za-z]{24}"\},"Based on the search results, example.com is reserved."\],"searchCount":1\}`, out)
	assert.NotContains(t, out, `"is_error":true`)
	assert.Contains(t, hooks, `"tool_response":{"durationSeconds":0,"query":"iana example domains"`)
	assert.Equal(t, 2, strings.Count(hooks, `"hook_event_name"`), "PreToolUse and PostToolUse")
	assert.NotContains(t, hooks, "mock_result", "the hooks are told the real tool's input")
	assert.NotContains(t, out, "mock_result", "nor does a frame of the call show it")
}

// TestT022_02_AllowedDomainsAreTheCallsOwn: a search that names the domains to keep is answered the same way; the
// mock searches nothing, so the domains narrow nothing (recorded: runs/web-search-tool).
// sr:proves web-search-tool/claude
func TestT022_02_AllowedDomainsAreTheCallsOwn(t *testing.T) {
	code, out, hooks := search(t, `{"query":"q","mode":"standard","allowed_domains":["iana.org"],"mock_result":{"links":[],"findings":"none"}}`)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `Links: []\n\nnone\n\n\nREMINDER`)
	assert.Contains(t, hooks, `"allowed_domains":["iana.org"]`)
}

// TestT022_03_ASearchTheMockCannotAnswerIsAnError: a search the script gives no result is an error result saying
// so, and a parameter the mock does not implement fails the run (adr/tool-calls-validated): the mock reaches no
// web.
func TestT022_03_ASearchTheMockCannotAnswerIsAnError(t *testing.T) {
	code, out, _ := search(t, `{"query":"q"}`)
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, "the mock searches nothing")
	code, out, _ = search(t, `{"query":"q","blocked_domains":["x.org"],"mock_result":{}}`)
	require.NotZero(t, code, out)
	assert.Contains(t, out, "blocked_domains")
}
