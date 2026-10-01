package e2e

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bashPostToolPayload is the PostToolUse payload of the Bash call whose
// tool_use_id starts with idPrefix.
func bashPostToolPayload(t *testing.T, log, idPrefix string) map[string]any {
	t.Helper()
	for _, p := range payloads(t, log) {
		if id, _ := p["tool_use_id"].(string); p["hook_event_name"] == "PostToolUse" && p["tool_name"] == "Bash" && len(id) >= len(idPrefix) && id[:len(idPrefix)] == idPrefix {
			return p
		}
	}
	require.FailNow(t, "no PostToolUse for the Bash call "+idPrefix)
	return nil
}

// TestT017_57_BackgroundBashPayloadAndReceipt: a Bash command run in the
// background answers at once, and its PostToolUse payload says so: the
// tool_response has an empty stdout and stderr, the flags false and a
// backgroundTaskId, and no backgroundEndsWithFinalResponse; the agent's receipt
// is the full recorded text naming the task and its output file (recorded:
// snapshots/runs/bgbash).
// sr:proves background-bash/claude
func TestT017_57_BackgroundBashPayloadAndReceipt(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	root := script(t, dir, "root", toolUse("bg", "Bash", `{"command":"sleep 8; echo BGDONE","description":"long sleep","run_in_background":true}`))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "bgp-1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, out)

	resp, _ := bashPostToolPayload(t, log, "bg")["tool_response"].(map[string]any)
	require.NotNil(t, resp)
	id, _ := resp["backgroundTaskId"].(string)
	assert.NotEmpty(t, id)
	assert.Equal(t, map[string]any{"stdout": "", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false, "backgroundTaskId": id}, resp,
		"an empty stdout, the task id, and no backgroundEndsWithFinalResponse outside a sub-agent")

	frames := toolResultFrames(out)
	require.NotEmpty(t, frames)
	assert.Regexp(t, regexp.MustCompile(`^Command running in background with ID: `+id+`\. Output is being written to: \S+/tasks/`+id+`\.output\. You will be notified when it completes\. To check interim output, use Read on that file path\.$`),
		frames[0]["content"], "the whole recorded receipt")
}

// TestT017_58_BackgroundBashInAForegroundSubagent: the same command run from a
// foreground sub-agent says the command ends with the sub-agent's final
// response: the PostToolUse payload (with the sub-agent's agent_id) adds
// backgroundEndsWithFinalResponse: true, and the receipt is the full recorded
// text warning that it is terminated when the sub-agent gives its final
// response (recorded: snapshots/runs/fg-subagent-bash).
// sr:proves foreground-subagent-bash-ends-with-response/claude
func TestT017_58_BackgroundBashInAForegroundSubagent(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	sub := script(t, dir, "sub", toolUse("sb", "Bash", `{"command":"sleep 30; echo SUBBG","description":"bgsleep","run_in_background":true}`))
	root := script(t, dir, "root", toolUse("ag", "Agent", `{"description":"bgrunner","prompt":"go","subagent_type":"general-purpose","script":"`+sub+`"}`))
	out, code := runInDir(t, dir, nil, "--script", root, "--session-id", "bgp-2", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, out)

	p := bashPostToolPayload(t, log, "sb")
	assert.NotEmpty(t, p["agent_id"], "the payload is the sub-agent's")
	resp, _ := p["tool_response"].(map[string]any)
	require.NotNil(t, resp)
	id, _ := resp["backgroundTaskId"].(string)
	assert.NotEmpty(t, id)
	assert.Equal(t, map[string]any{"stdout": "", "stderr": "", "interrupted": false, "isImage": false, "noOutputExpected": false,
		"backgroundTaskId": id, "backgroundEndsWithFinalResponse": true}, resp)

	// the sub-agent's own tool results are in its transcript, not the main stream
	files, _ := filepath.Glob(filepath.Join(cfg, "projects", "*", "bgp-2", "subagents", "agent-*.jsonl"))
	require.Len(t, files, 1)
	var receipt string
	for _, r := range readRecs(t, files[0]) {
		var m struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Raw), &m))
		blocks, _ := m.Message.Content.([]any) // a plain prompt's content is a string
		for _, b := range blocks {
			c, _ := b.(map[string]any)
			if s, _ := c["content"].(string); strings.HasPrefix(s, "Command running in background") {
				receipt = s
			}
		}
	}
	assert.Regexp(t, regexp.MustCompile(`^Command running in background with ID: `+id+`\. Output is being written to: \S+/tasks/`+id+`\.output\. If it exits while you are still working you will be notified, but it is terminated when you give your final response and no notification can follow that — so do not end your turn to wait for it; if you need its result, wait for it before giving your final response\. To check interim output, use Read on that file path\.$`),
		receipt, "the whole recorded receipt")
}
