package e2e

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The compaction hooks carry the common fields too: the session's id, its transcript and its directory,
// and no agent (recorded: runs/manual-compaction-auto, PreCompact and PostCompact payloads).
// sr:proves hook-common-payload/codex
func TestCompactionHooksCarryTheCommonFields(t *testing.T) {
	rec, got := replayCompacting(t, "manual-compaction-auto")
	require.Equal(t, 0, got.Code, got.Stderr)
	sid := sessionIDOf(t, got)
	for name, log := range map[string][]map[string]any{"recorded": jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))), "mock": got.hookLog()} {
		seen := 0
		for _, p := range log {
			ev, _ := p["hook_event_name"].(string)
			if ev != "PreCompact" && ev != "PostCompact" {
				continue
			}
			seen++
			for _, k := range []string{"session_id", "transcript_path", "cwd"} {
				assert.NotEmpty(t, p[k], "%s: %s carries %s", name, ev, k)
			}
			assert.NotContains(t, p, "agent_id", name)
			if name == "mock" {
				assert.Equal(t, sid, p["session_id"], ev)
				assert.Equal(t, got.Repo, p["cwd"], ev)
			}
		}
		assert.NotZero(t, seen, name+": the run fired compaction hooks")
	}
}

// The stream shows a command as the shell invocation that ran it: the recorded runs show `/bin/zsh -lc
// '<command>'` (the login shell Codex runs it by), and the mock's is a `<shell> -c '<command>'` line of
// the same form, never the bare command (runs/stops, runs/hook-exit-codes).
// sr:proves noninteractive-run/codex
func TestTheStreamShowsACommandAsTheShellInvocationThatRanIt(t *testing.T) {
	form := regexp.MustCompile(`^/bin/(ba|z)?sh -l?c .+$`)
	for _, run := range []string{"stops", "hook-exit-codes"} {
		rec := loadRecording(t, run)
		n := 0
		for _, e := range jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl"))) {
			if item, _ := e["item"].(map[string]any); item["type"] == "command_execution" {
				n++
				assert.Regexp(t, `^/bin/zsh -lc .+$`, item["command"], "recorded: "+run)
			}
		}
		assert.NotZero(t, n, run)
	}
	got := execMock(t, scenario{Script: callThenResult, Prompt: "go", Env: withCalls(t, "echo hi")})
	require.Equal(t, 0, got.Code, got.Stderr)
	var seen []string
	for _, e := range got.stream() {
		if item, _ := e["item"].(map[string]any); item["type"] == "command_execution" {
			seen = append(seen, item["command"].(string))
		}
	}
	require.Len(t, seen, 2, "started and completed")
	for _, c := range seen {
		assert.Regexp(t, form, c)
		assert.Contains(t, c, "'echo hi'", "the command, quoted as an argument of the shell")
	}
}

// The Interrupt hook's payload carries the common fields as well (recorded: runs/interrupt-hook).
// sr:proves hook-common-payload/codex
func TestTheInterruptHookCarriesTheCommonFields(t *testing.T) {
	rec := loadRecording(t, "interrupt-hook")
	pick := func(log []map[string]any) map[string]any {
		for _, p := range log {
			if p["hook_event_name"] == "Interrupt" {
				return p
			}
		}
		return nil
	}
	recorded := pick(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.NotNil(t, recorded)
	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "sleep 30"), InterruptOnCommand: true,
	})
	mock := pick(got.hookLog())
	require.NotNil(t, mock)
	for _, k := range []string{"session_id", "transcript_path", "cwd"} {
		assert.NotEmpty(t, recorded[k], "recorded: "+k)
		assert.NotEmpty(t, mock[k], "mock: "+k)
	}
	assert.Equal(t, sessionIDOf(t, got), mock["session_id"])
	assert.Equal(t, got.Repo, mock["cwd"])
	assert.Equal(t, keysOf(recorded), keysOf(mock), "the same fields")
}
