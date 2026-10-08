package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/session-resume: a session that ran a command, then
// resumed by its id from a directory it was not begun in, and asked what the
// command printed.

// resumeScript runs `echo ORIGINAL-WORD` for the first prompt; for the prompt
// that asks about it, it answers with what the session's rollout holds.
const resumeScript = `#!/bin/sh
case "$A10N_MOCK_PROMPT" in
*earlier*)
  w=$(grep -o 'ORIGINAL-WORD' "$A10N_MOCK_SESSION_FILE" | head -n1)
  printf '%s\n' "{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"$w\"}]}}" "{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"$w\"}"
  exit 0 ;;
esac
if ! grep -q function_call_output "$A10N_MOCK_SESSION_FILE"; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_1","name":"Bash","input":{"command":"echo ORIGINAL-WORD"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"OK"}]}}' '{"type":"result","subtype":"success","result":"OK"}'
`

// resumeIn runs `exec resume <id> <prompt>` in dir, against the run's home and
// hook log.
func resumeIn(t *testing.T, first result, dir, id, prompt string) result {
	t.Helper()
	root := filepath.Dir(first.Repo)
	cmd := exec.Command(mockBinary, "exec", "--json", "--skip-git-repo-check", "--dangerously-bypass-hook-trust", "-s", "workspace-write", "--script", filepath.Join(root, "scenario.sh"),
		"-m", "mock-model", "resume", id, prompt)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CODEX_HOME="+first.Home, "TMPDIR="+first.Tmp, "HOOK_LOG="+filepath.Join(first.Tmp, "hook.log"))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), errb.String())
	return result{Repo: dir, Home: first.Home, Tmp: first.Tmp, Stdout: out.String(), Stderr: errb.String()}
}

// A session is resumed by its id, from a directory other than the one it began
// in: it continues in the same rollout file (no new one, the new records after
// the old), under the same id, and the agent sees what happened before. The
// start hook fires again with source "resume", in the new directory, and no
// sub-agent hook fires (runs/session-resume).
// sr:proves session-resume/codex
// sr:proves session-start-hook/codex
// sr:proves noninteractive-run/codex
func TestResumeByIDContinuesInTheRolloutFromAnotherDirectory(t *testing.T) {
	rec := loadRecording(t, "session-resume")
	first := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")), Script: resumeScript,
		Prompt: strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
	})
	require.Equal(t, 0, first.Code, first.Stderr)
	id, _ := first.stream()[0]["thread_id"].(string)
	require.NotEmpty(t, id)
	before := first.rollout(t)

	elsewhere, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	got := resumeIn(t, first, elsewhere, id, strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "then-01-prompt.txt"))))

	var files []string
	require.NoError(t, filepath.Walk(filepath.Join(first.Home, "sessions"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, p)
		}
		return nil
	}))
	require.Len(t, files, 1, "the resumed session writes to its own rollout, not a new one")
	assert.True(t, strings.HasPrefix(first.rollout(t), before), "the old records stay, the new ones follow")
	assert.Greater(t, len(first.rollout(t)), len(before))
	assert.Equal(t, id, got.stream()[0]["thread_id"], "the same session id")

	var said []string
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "agent_message" {
			said = append(said, item["text"].(string))
		}
	}
	assert.Equal(t, []string{"ORIGINAL-WORD"}, said, "the agent has the earlier conversation")

	// the hooks fired, as recorded: event, and for SessionStart how it began
	wantEvents := []string{}
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		ev, _ := l["hook_event_name"].(string)
		src, _ := l["source"].(string)
		wantEvents = append(wantEvents, ev+" "+src)
		assert.NotContains(t, ev, "Subagent")
	}
	var gotEvents []string
	for _, l := range first.hookLog() {
		ev, _ := l["hook_event_name"].(string)
		src, _ := l["source"].(string)
		gotEvents = append(gotEvents, ev+" "+src)
		assert.NotContains(t, ev, "Subagent")
		if sid, _ := l["session_id"].(string); sid != "" {
			assert.Equal(t, id, sid)
		}
	}
	assert.Equal(t, wantEvents, gotEvents)
	hooks := first.hookLog()
	assert.Equal(t, elsewhere, hooks[len(hooks)-1]["cwd"], "the session's directory is where it was resumed")
	assert.NotEqual(t, elsewhere, hooks[0]["cwd"])
	assert.Equal(t, sortedHookLines(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))), sortedHookLines(hooks))
}
