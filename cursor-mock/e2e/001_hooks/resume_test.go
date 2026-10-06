package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/session-resume: a session that ran a command, then was
// resumed by its id from another directory, and then from its own, each time
// asked what the command printed.

// resumeScript runs `echo ORIGINAL-WORD` for the first prompt; for the prompt
// that asks about it, it answers with what the session's transcript holds.
const resumeScript = `#!/bin/sh
case "$A10N_MOCK_PROMPT" in
*earlier*)
  w=$(grep -o 'ORIGINAL-WORD' "$A10N_MOCK_SESSION_FILE" | head -n1)
  w=${w:-NO-HISTORY}
  printf '%s\n' "{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"$w\"}]}}" "{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"$w\"}"
  exit 0 ;;
esac
if ! grep -q '"type":"tool_use"' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_1","name":"Bash","input":{"command":"echo ORIGINAL-WORD"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"OK"}]}}' '{"type":"result","subtype":"success","is_error":false,"result":"OK"}'
`

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// workspace is a fresh directory with the recorded run's hooks installed.
func workspace(t *testing.T, setup string) string {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	return ws
}

// agent runs the mock in ws, with extra flags, and returns its stream's lines.
func agent(t *testing.T, ws, home, hookLog, script, prompt string, flags ...string) []map[string]any {
	t.Helper()
	args := append([]string{"-p", "--force", "--trust", "--output-format", "stream-json"}, flags...)
	cmd := exec.Command(binary, append(args, prompt)...)
	cmd.Dir = ws
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + hookLog, "A10N_MOCK_SCRIPT=" + script}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), errb.String())
	path := filepath.Join(t.TempDir(), "stream")
	require.NoError(t, os.WriteFile(path, out.Bytes(), 0o644))
	return readJSONL(t, path)
}

// results are the texts of the result frames, one per run.
func results(frames ...[]map[string]any) (out []string) {
	for _, fs := range frames {
		for _, f := range fs {
			if f["type"] == "result" {
				out = append(out, f["result"].(string))
			}
		}
	}
	return
}

// shape is what a transcript holds: how many user messages, and how many
// turn_ended lines, and whether the last line is one.
func shape(t *testing.T, path string) (users, ends int, endsLast bool) {
	t.Helper()
	lines := readJSONL(t, path)
	for _, l := range lines {
		if l["role"] == "user" {
			users++
		}
		if l["type"] == "turn_ended" {
			ends++
		}
	}
	return users, ends, len(lines) > 0 && lines[len(lines)-1]["type"] == "turn_ended"
}

// A session is resumed by its id: from its own directory it continues in its
// existing transcript (the new messages after the old, one closing line) and the
// agent has the earlier conversation; from another directory it keeps the id and
// the session's hooks fire there, but cursor-agent carries no conversation to it,
// and the transcript it makes there holds only the new messages. A resumed
// session fires its end hook and no start hook, and no sub-agent hook
// (runs/session-resume).
// sr:proves session-resume/cursor
// sr:proves session-start-hook/cursor
func TestResumeByIDContinuesInTheTranscriptOfItsDirectoryOnly(t *testing.T) {
	setup, _, _, firstPrompt := recording(t, "session-resume")
	secondPrompt := strings.TrimSpace(readFileOrFail(t, filepath.Join(setup, "then-01-prompt.txt")))
	ws, other, home := workspace(t, setup), workspace(t, setup), t.TempDir()
	script := filepath.Join(t.TempDir(), "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(resumeScript), 0o755))
	hookLog := filepath.Join(t.TempDir(), "payloads.jsonl")

	first := agent(t, ws, home, hookLog, script, firstPrompt)
	id, _ := first[0]["session_id"].(string)
	require.NotEmpty(t, id)
	elsewhere := agent(t, other, home, hookLog, script, secondPrompt, "--resume", id)
	home2 := agent(t, ws, home, hookLog, script, secondPrompt, "--resume", id)
	assert.Equal(t, []string{"OK", "NO-HISTORY", "ORIGINAL-WORD"}, results(first, elsewhere, home2))
	for _, fs := range [][]map[string]any{elsewhere, home2} {
		assert.Equal(t, id, fs[0]["session_id"], "a resumed session keeps its id")
	}

	// the hooks: a start hook for the session's beginning only, an end hook for every
	// run (the payloads compared with the recording are the generated replay's,
	// e2e/003_replay: the recorded model ran its command twice, the script once)
	var got []string
	cwds := map[string]bool{}
	for _, m := range readJSONL(t, hookLog) {
		if ev, ok := m["hook_event_name"].(string); ok {
			got = append(got, ev)
			assert.Equal(t, id, m["session_id"])
			roots, _ := m["workspace_roots"].([]any)
			cwds[roots[0].(string)] = true
		}
	}
	assert.Equal(t, []string{"sessionStart", "afterShellExecution", "sessionEnd", "sessionEnd", "sessionEnd"}, got, "one start hook, for the session's beginning only; an end hook for every run")
	assert.Equal(t, map[string]bool{ws: true, other: true}, cwds, "the hooks of the directory it was resumed in fire")

	// the transcripts, as recorded: the session's own continues, the other's is new
	project := func(dir string) string {
		return filepath.Join(home, ".cursor", "projects", nonAlnum.ReplaceAllString(strings.TrimPrefix(dir, "/"), "-"), "agent-transcripts", id, id+".jsonl")
	}
	sample := newestSample(t, "session-resume")
	recorded := func(dir string) string {
		files, _ := filepath.Glob(filepath.Join(sample, "transcript", dir, "*", "*.jsonl"))
		require.Len(t, files, 1)
		return files[0]
	}
	for name, p := range map[string][2]string{"its own directory": {recorded("repo"), project(ws)}, "another directory": {recorded("elsewhere"), project(other)}} {
		wu, we, wl := shape(t, p[0])
		gu, ge, gl := shape(t, p[1])
		assert.Equal(t, []any{wu, we, wl}, []any{gu, ge, gl}, name)
	}
	users, ends, last := shape(t, project(ws))
	assert.Equal(t, []any{2, 1, true}, []any{users, ends, last}, "the continued transcript: both prompts, one closing line")
}

func readFileOrFail(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
