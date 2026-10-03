package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordedHookSeq is [event, source, agent_type, trigger] of each hook in a recording's
// events.jsonl, from the n-th SessionStart on.
func recordedHookSeq(t *testing.T, run string, fromStart int) [][4]string {
	t.Helper()
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/"+run+"/samples/*/events.jsonl"))
	require.NoError(t, err)
	var out [][4]string
	starts := 0
	for _, l := range strings.Split(string(raw), "\n") {
		var e struct {
			Hook    string
			Payload map[string]any
		}
		if json.Unmarshal([]byte(l), &e) != nil || e.Hook == "" {
			continue
		}
		if e.Hook == "SessionStart" {
			starts++
		}
		if starts < fromStart {
			continue
		}
		field := func(k string) string { v, _ := e.Payload[k].(string); return v }
		out = append(out, [4]string{e.Hook, field("source"), field("agent_type"), field("trigger")})
	}
	return out
}

func hookSeqOf(t *testing.T, log string, from int) (out [][4]string) {
	for _, p := range payloads(t, log)[from:] {
		field := func(k string) string { v, _ := p[k].(string); return v }
		out = append(out, [4]string{p["hook_event_name"].(string), field("source"), field("agent_type"), field("trigger")})
	}
	return
}

// TestT017_86_ResumeThenCompactSequenceAsRecorded: a resumed session that runs
// /compact is the one place a resume sees a sub-agent hook: the recorded compact
// run fires SessionStart(resume), PreCompact, SubagentStop (the summarizer, agent
// type ""), SessionStart(compact), PostCompact and SessionEnd, in that order,
// and the mock's does the same; a resume that does not compact fires no
// sub-agent hook.
// sr:proves session-resume/claude
func TestT017_86_ResumeThenCompactSequenceAsRecorded(t *testing.T) {
	want := recordedHookSeq(t, "compact", 2)
	require.Equal(t, [][4]string{
		{"SessionStart", "resume", "", ""}, {"PreCompact", "", "", "manual"}, {"SubagentStop", "", "", ""},
		{"SessionStart", "compact", "", ""}, {"PostCompact", "", "", "manual"}, {"SessionEnd", "", "", ""},
	}, want, "recorded")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	events := map[string]string{}
	for _, ev := range []string{"SessionStart", "PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "SessionEnd"} {
		events[ev] = h
	}
	settings(t, dir, events)
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a", toolUse("b1", "Bash", `{"command":"true"}`)), "--session-id", "rc-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "first")
	require.Equal(t, 0, code, out)
	started := len(payloads(t, log))

	out, code = runInDir(t, dir, nil, "--script", script(t, dir, "b"), "--resume", "rc-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "again")
	require.Equal(t, 0, code, out)
	for _, hk := range hookSeqOf(t, log, started) {
		assert.NotContains(t, []string{"SubagentStart", "SubagentStop"}, hk[0], "a resume that does not compact fires no sub-agent hook")
	}
	started = len(payloads(t, log))

	sc := script(t, dir, "c", `{"type":"compact","summary":"s @MARK@","trigger":"manual"}`)
	out, code = runInDir(t, dir, nil, "--script", sc, "--resume", "rc-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "/compact")
	require.Equal(t, 0, code, out)
	assert.Equal(t, want, hookSeqOf(t, log, started))
}

// TestT017_87_NoSessionPersistenceStillFiresTheHooks: with --no-session-persistence
// the session's hooks fire as ever, as the recorded run's did: SessionStart
// (startup), UserPromptSubmit, Stop, SessionEnd.
// sr:proves session-resume/claude
func TestT017_87_NoSessionPersistenceStillFiresTheHooks(t *testing.T) {
	want := recordedHookSeq(t, "no-session-persistence", 1)
	require.Equal(t, [][4]string{{"SessionStart", "startup", "", ""}, {"UserPromptSubmit", "", "", ""}, {"Stop", "", "", ""}, {"SessionEnd", "", "", ""}}, want, "recorded")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "UserPromptSubmit": h, "Stop": h, "SessionEnd": h})
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "a"), "--session-id", "np-2", "--no-session-persistence",
		"--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, out)
	assert.Equal(t, want, hookSeqOf(t, log, 0))
	assert.NoFileExists(t, transcriptPath(t, cfg, dir, "np-2"))
}
