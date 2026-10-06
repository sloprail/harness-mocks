package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/nested-subagents-background: the main agent starts a
// sub-agent A, which starts a sub-agent B with run_in_background true and ends
// without waiting for it (A's report is STARTED); the run goes on until B is
// done.

// No hook fires after a Task call: the recorded hooks include postToolUse and
// postToolUseFailure, and neither fired for any Task call of the recorded runs.
// What the hook of a background Task call saw, recorded, is the call as made,
// with run_in_background true, and the mock's says the same.
// sr:proves nested-subagents/cursor
func TestATaskCallFiresOnlyPreToolUseAndABackgroundOneIsSeenAsMade(t *testing.T) {
	sample := newestSample(t, "nested-subagents-background")
	var recTask []map[string]any
	for _, h := range readJSONL(t, filepath.Join(sample, "payloads.jsonl")) {
		if h["tool_name"] == "Task" {
			recTask = append(recTask, h)
			assert.Equal(t, "preToolUse", h["hook_event_name"], "no hook after a Task call")
		}
	}
	require.Len(t, recTask, 2)
	in, _ := recTask[1]["tool_input"].(map[string]any)
	assert.Equal(t, true, in["run_in_background"], "the sub-agent's call asks for a background one")

	// the mock: the same chain, B asked for in the background
	ws, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	scratch, home := t.TempDir(), t.TempDir()
	setup, _, _, _ := recording(t, "nested-subagents-background")
	copyFile(t, filepath.Join(setup, "hooks.json"), filepath.Join(ws, ".cursor", "hooks.json"), 0o644)
	copyFile(t, filepath.Join(setup, "hook.sh"), filepath.Join(ws, ".cursor", "hooks", "hook.sh"), 0o755)
	leaf := filepath.Join(scratch, "leaf.sh")
	require.NoError(t, os.WriteFile(leaf, []byte(strings.Replace(layerScript(""), "echo LEAF", "sleep 1; echo LEAF", 1)), 0o755))
	a := filepath.Join(scratch, "a.sh")
	require.NoError(t, os.WriteFile(a, []byte(strings.ReplaceAll(layerScript(leaf), `"prompt":"go"`, `"prompt":"go","run_in_background":true`)), 0o755))
	main := filepath.Join(scratch, "main.sh")
	require.NoError(t, os.WriteFile(main, []byte(layerScript(a)), 0o755))
	log := filepath.Join(scratch, "payloads.jsonl")
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", main, "go")
	cmd.Dir, cmd.Env = ws, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "HOOK_LOG=" + log}
	began := time.Now()
	out, err := cmd.Output()
	require.NoError(t, err, "the mock failed: %s", out)
	lasted := time.Since(began)

	// the shell commands a sub-agent runs fire their hooks in its own session,
	// not the main agent's
	shellSessions := func(lines []map[string]any) (main string, shell []string) {
		for _, h := range lines {
			if h["hook_event_name"] == "preToolUse" && h["tool_name"] == "Task" && main == "" {
				main = h["session_id"].(string)
			}
			if h["hook_event_name"] == "preToolUse" && (h["tool_name"] == "Shell" || h["tool_name"] == "Bash") {
				shell = append(shell, h["session_id"].(string))
			}
		}
		return
	}
	recMain, recShell := shellSessions(readJSONL(t, filepath.Join(sample, "payloads.jsonl")))
	require.NotEmpty(t, recShell, "the recorded sub-agent ran a shell command")
	for _, s := range recShell {
		assert.NotEqual(t, recMain, s, "recorded: not the main agent's session")
	}
	gotMain, gotShell := shellSessions(readJSONL(t, log))
	require.NotEmpty(t, gotShell)
	for _, s := range gotShell {
		assert.NotEqual(t, gotMain, s, "mock: not the main agent's session")
	}

	var got []map[string]any
	for _, h := range readJSONL(t, log) {
		if h["tool_name"] == "Task" {
			got = append(got, h)
			assert.Equal(t, "preToolUse", h["hook_event_name"], "no hook after a Task call")
		}
	}
	require.Len(t, got, 2)
	gin, _ := got[1]["tool_input"].(map[string]any)
	assert.Equal(t, true, gin["run_in_background"], "the hook sees the call as made")
	m, err := filepath.Glob(filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts", "*", "*.jsonl"))
	require.NoError(t, err)
	assert.Len(t, m, 3, fmt.Sprint(m))

	// the sub-agent does not wait for the background sub-agent it launched: it
	// has given its report (its transcript is done) while the other, which
	// sleeps, is still running, and the run goes on until that one is done
	// (recorded: A ends with STARTED while B runs, and the run lasts until B is done)
	mtime := func(session string) int64 {
		files, err := filepath.Glob(filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts", session, "*.jsonl"))
		require.NoError(t, err)
		require.Len(t, files, 1)
		info, err := os.Stat(files[0])
		require.NoError(t, err)
		return info.ModTime().UnixNano()
	}
	assert.Less(t, mtime(got[1]["session_id"].(string)), mtime(gotShell[0]),
		"the launching sub-agent ended before the background one it launched")

	// the run lasts until the background sub-agent is done, and all of it is
	// recorded to its end: its command's hooks and the session's end come after
	// the launching sub-agent's report, and its end is reported on the stream
	// (recorded: runs/nested-subagents-background, B's afterShellExecution and
	// postToolUse, then sessionEnd, and a task_notification naming B)
	assert.GreaterOrEqual(t, lasted.Milliseconds(), int64(1000), "the run lasts at least the background sub-agent's sleep")
	order := func(lines []map[string]any, bSession string) (b []string, end int) {
		end = -1
		for i, h := range lines {
			e, _ := h["hook_event_name"].(string)
			if h["session_id"] == bSession && (e == "beforeShellExecution" || e == "afterShellExecution" || e == "postToolUse") {
				b = append(b, e)
			}
			if e == "sessionEnd" {
				end = i
			}
		}
		return
	}
	recLines := readJSONL(t, filepath.Join(sample, "payloads.jsonl"))
	recB, recEnd := order(recLines, recShell[0])
	assert.Equal(t, []string{"beforeShellExecution", "afterShellExecution", "postToolUse"}, recB, "recorded: the background sub-agent's command's hooks")
	gotLines := readJSONL(t, log)
	gotB, gotEnd := order(gotLines, gotShell[0])
	assert.Equal(t, recB, gotB, "mock: the background sub-agent's command's hooks")
	for name, lines := range map[string][]map[string]any{"recorded": recLines, "mock": gotLines} {
		bSession, end := recShell[0], recEnd
		if name == "mock" {
			bSession, end = gotShell[0], gotEnd
		}
		last := -1
		for i, h := range lines {
			if h["session_id"] == bSession && h["hook_event_name"] == "postToolUse" {
				last = i
			}
		}
		assert.Greater(t, end, last, name+": sessionEnd comes after the background sub-agent's last hook")
	}
	notes := func(frames []map[string]any, b string) (n int) {
		for _, f := range frames {
			if f["subtype"] == "task_notification" && f["task_id"] == b && f["status"] == "success" {
				n++
			}
		}
		return
	}
	recStream := readJSONL(t, filepath.Join(sample, "stream.jsonl"))
	assert.Equal(t, 1, notes(recStream, recShell[0]), "recorded: one task_notification naming the background sub-agent")
	assert.Equal(t, 1, notes(readJSONLText(t, string(out)), gotShell[0]), "mock: the same")
	files, err := filepath.Glob(filepath.Join(home, ".cursor", "projects", "*", "agent-transcripts", gotShell[0], "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	b, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Contains(t, string(b), "LEAF", "the background sub-agent's reply is in its transcript")
}
