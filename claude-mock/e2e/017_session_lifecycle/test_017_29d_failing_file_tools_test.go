package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_29d_FailingEditAndWrite: a file tool that ran and failed, an Edit of a file that is not
// there and a Write whose path is under a file, answers the call as an error (is_error) and fires
// PostToolUseFailure, with the error text the agent got and duration_ms, not PostToolUse. An Edit
// whose old_string is not in the file never ran: it is an error with no hook at all (recorded:
// snapshots/runs/file-tools). The wording of the Write's error is the mock's own; that the failure
// hook fires for a file tool's error is recorded for a Read (snapshots/runs/tool-errors), see
// TestT017_29b_FailingRead.
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure-input
// sr:proves tool-failure-hook/claude
func TestT017_29d_FailingEditAndWrite(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PostToolUse": h, "PostToolUseFailure": h})
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644))
	sc := script(t, dir, "s",
		toolUse("r1", "Read", `{"file_path":"a.txt"}`),
		toolUse("n1", "Edit", `{"file_path":"a.txt","old_string":"absent text","new_string":"x"}`),
		toolUse("e1", "Edit", `{"file_path":"missing.txt","old_string":"a","new_string":"x"}`),
		toolUse("w1", "Write", `{"file_path":"a.txt/under/b.txt","content":"x"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "fe-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "fe-1"))
	ps := payloads(t, log)
	require.Len(t, ps, 3, "the Read's PostToolUse, then one failure hook for each call that ran and failed")
	refused, _ := toolResultOf(t, recs, "n1turn-s-b")
	assert.Equal(t, true, refused["is_error"], "an Edit of a string that is not there is an error, and has no hook")
	assert.Equal(t, "PostToolUse", ps[0]["hook_event_name"])
	for i, tc := range []struct{ id, tool string }{{"e1turn-s-c", "Edit"}, {"w1turn-s-d", "Write"}} {
		block, _ := toolResultOf(t, recs, tc.id)
		assert.Equal(t, true, block["is_error"], tc.tool)
		p := ps[i+1]
		assert.Equal(t, "PostToolUseFailure", p["hook_event_name"], tc.tool)
		assert.Equal(t, tc.tool, p["tool_name"])
		assert.NotEmpty(t, p["error"], tc.tool)
		d, ok := p["duration_ms"].(float64)
		assert.True(t, ok && d >= 0, "%s: duration_ms is a number of milliseconds: %v", tc.tool, p["duration_ms"])
		assert.NotContains(t, p, "tool_response", tc.tool)
	}
}

// failureDuration runs one failing Bash under a PostToolUseFailure hook (and a PreToolUse hook, when
// pre is not "") and returns the failure's duration_ms.
func failureDuration(t *testing.T, id, command, pre string) float64 {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "failure.log")
	events := map[string]string{"PostToolUseFailure": payloadLogger(t, dir, "failure.sh", log, "")}
	if pre != "" {
		events["PreToolUse"] = write(t, filepath.Join(dir, "pre.sh"), "#!/bin/sh\ncat >/dev/null\n"+pre+"\n", 0o755)
	}
	settings(t, dir, events)
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", toolUse("b1", "Bash", `{"command":"`+command+`"}`)),
		"--session-id", id, "--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	require.Equal(t, "PostToolUseFailure", ps[0]["hook_event_name"])
	d, ok := ps[0]["duration_ms"].(float64)
	require.True(t, ok, "duration_ms is a number: %v", ps[0]["duration_ms"])
	return d
}

// TestT017_29e_FailureDurationIsTheCallsOwnTime: a failing command's PostToolUseFailure carries the
// time the call took: a command that sleeps 0.3 seconds before it fails took at least 300 ms.
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure-input
// sr:proves tool-failure-hook/claude
func TestT017_29e_FailureDurationIsTheCallsOwnTime(t *testing.T) {
	assert.GreaterOrEqual(t, failureDuration(t, "fd-1", "sleep 0.3; exit 1", ""), 300.0)
}

// TestT017_29f_FailureDurationExcludesPreToolUseHooks: as for PostToolUse (T017_37), the failure's
// duration_ms leaves out the time of a PreToolUse hook: a half-second hook before a command that fails
// at once does not show in it.
// sr:docs https://code.claude.com/docs/en/hooks#posttoolusefailure-input
// sr:proves tool-failure-hook/claude
func TestT017_29f_FailureDurationExcludesPreToolUseHooks(t *testing.T) {
	assert.Less(t, failureDuration(t, "fd-2", "exit 1", "sleep 0.5"), 300.0, "the half-second PreToolUse hook is not part of the call's time")
}
