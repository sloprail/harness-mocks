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

// hookIDs is [event, source-or-"", session_id] of each SessionStart and SessionEnd
// of a recording's events.jsonl, in order; the run's own id reads <SESSION_ID>.
func recordedSessionHooks(t *testing.T) [][3]string {
	t.Helper()
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/forkresume/samples/*/events.jsonl"))
	require.NoError(t, err)
	var out [][3]string
	for _, l := range strings.Split(string(raw), "\n") {
		var e struct {
			Hook    string
			Payload map[string]any
		}
		if json.Unmarshal([]byte(l), &e) != nil || (e.Hook != "SessionStart" && e.Hook != "SessionEnd") {
			continue
		}
		src, _ := e.Payload["source"].(string)
		out = append(out, [3]string{e.Hook, src, e.Payload["session_id"].(string)})
	}
	return out
}

// TestT017_69_ForkResumeSequenceAsRecorded replays the forkresume run's five
// steps against the mock: a session, a resume of it, a fork of it under a new
// id, --fork-session with nothing to fork (a plain new session), and a resume
// of the ORIGINAL after all that. The hooks carry what the recording's did:
// the fork's SessionStart and SessionEnd name the fork's session id and
// source "fork", --fork-session alone starts a session from scratch (source
// "startup", no history), and resuming the original afterwards says
// source "resume" under the original id and appends to the original's file
// alone, leaving the fork's file as it was.
// sr:proves session-fork/claude
func TestT017_69_ForkResumeSequenceAsRecorded(t *testing.T) {
	const a, b, c = "00000000-0000-4000-8000-0000000000a1", "00000000-0000-4000-8000-0000000000b2", "00000000-0000-4000-8000-0000000000c3"
	real := recordedSessionHooks(t)
	require.Len(t, real, 10)
	want := [][3]string{
		{"SessionStart", "startup", "<SESSION_ID>"}, {"SessionEnd", "", "<SESSION_ID>"},
		{"SessionStart", "resume", "<SESSION_ID>"}, {"SessionEnd", "", "<SESSION_ID>"},
		{"SessionStart", "fork", b}, {"SessionEnd", "", b},
		{"SessionStart", "startup", c}, {"SessionEnd", "", c},
		{"SessionStart", "resume", "<SESSION_ID>"}, {"SessionEnd", "", "<SESSION_ID>"},
	}
	assert.Equal(t, want, real, "the recorded sequence")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "SessionEnd": h})
	run := func(name string, args ...string) {
		t.Helper()
		out, code := runInDir(t, dir, nil, append([]string{"--script", script(t, dir, name, toolUse("b1", "Bash", `{"command":"true"}`)),
			"--project-dir", dir, "--config-dir", cfg, "-p", "go " + name}, args...)...)
		require.Equal(t, 0, code, out)
	}
	run("one", "--session-id", a)
	run("two", "--resume", a)
	run("three", "--resume", a, "--fork-session", "--session-id", b)
	forkBefore, err := os.ReadFile(transcriptPath(t, cfg, dir, b))
	require.NoError(t, err)
	run("four", "--fork-session", "--session-id", c)
	run("five", "--resume", a)

	var got [][3]string
	for _, p := range payloads(t, log) {
		src, _ := p["source"].(string)
		got = append(got, [3]string{p["hook_event_name"].(string), src, p["session_id"].(string)})
	}
	require.Len(t, got, 10)
	for i := range want {
		wantID := want[i][2]
		if wantID == "<SESSION_ID>" {
			wantID = a
		}
		assert.Equal(t, [3]string{want[i][0], want[i][1], wantID}, got[i], "hook %d", i)
	}

	// --fork-session without --resume: a fresh session, none of the original's history.
	plain, err := os.ReadFile(transcriptPath(t, cfg, dir, c))
	require.NoError(t, err)
	assert.NotContains(t, string(plain), "go one", "nothing was forked into it")
	assert.Contains(t, string(plain), "go four")

	// Resuming the original after the fork appends to the original's file only.
	forkAfter, err := os.ReadFile(transcriptPath(t, cfg, dir, b))
	require.NoError(t, err)
	assert.Equal(t, string(forkBefore), string(forkAfter), "the fork's file is untouched by resuming the original")
	orig, err := os.ReadFile(transcriptPath(t, cfg, dir, a))
	require.NoError(t, err)
	assert.Contains(t, string(orig), "go five", "the original continues in its own file")
	assert.NotContains(t, string(orig), "go three", "and holds nothing of the fork's turn")
}
