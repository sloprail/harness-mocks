package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	origID = "00000000-0000-4000-8000-0000000000a1"
	forkID = "00000000-0000-4000-8000-0000000000a2"
)

// streamIDs are the session_id of every frame of a stream that names one.
func streamIDs(t *testing.T, out string) []string {
	t.Helper()
	var ids []string
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil {
			if id, ok := f["session_id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func sessionSetup(t *testing.T, events ...string) (dir, cfg, log string) {
	t.Helper()
	dir = t.TempDir()
	cfg, log = filepath.Join(dir, "config"), filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	hooks := map[string]string{}
	for _, e := range events {
		hooks[e] = h
	}
	settings(t, dir, hooks)
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", origID,
		"--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "first")
	require.Equal(t, 0, code, out)
	write(t, log, "", 0o644)
	return dir, cfg, log
}

// A fork's own hooks, not only its SessionStart, carry the fork's session id,
// not the id of the session it branched from (runs/forkresume,
// resume-continue-fork).
// sr:proves session-fork/claude
func TestT017_83_ForkHooksCarryTheForksSessionID(t *testing.T) {
	dir, cfg, log := sessionSetup(t, "SessionStart", "UserPromptSubmit", "Stop", "SessionEnd")
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s2"), "--resume", origID, "--fork-session", "--session-id", forkID,
		"--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "forked")
	require.Equal(t, 0, code, out)
	var events []any
	for _, p := range payloads(t, log) {
		events = append(events, p["hook_event_name"])
		assert.Equal(t, forkID, p["session_id"], p["hook_event_name"])
		assert.True(t, strings.HasSuffix(p["transcript_path"].(string), "/"+forkID+".jsonl"), "%v names the fork's own file: %v", p["hook_event_name"], p["transcript_path"])
	}
	assert.Equal(t, []any{"SessionStart", "UserPromptSubmit", "Stop", "SessionEnd"}, events)
	for _, id := range streamIDs(t, out) {
		assert.Equal(t, forkID, id, "every frame of the fork's stream")
	}
}

// A resume by id keeps the session's id on every frame of the stream and on
// every hook payload (runs/forkresume).
// sr:proves session-resume/claude
func TestT017_83_ResumeKeepsTheSessionID(t *testing.T) {
	dir, cfg, log := sessionSetup(t, "SessionStart", "UserPromptSubmit", "Stop", "SessionEnd")
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s2"), "--resume", origID,
		"--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "again")
	require.Equal(t, 0, code, out)
	ids := streamIDs(t, out)
	require.NotEmpty(t, ids)
	for _, id := range ids {
		assert.Equal(t, origID, id, "every frame of the resumed stream")
	}
	for _, p := range payloads(t, log) {
		assert.Equal(t, origID, p["session_id"], p["hook_event_name"])
	}
}

// An unknown session's resume fails on its own, with no env var: the
// SessionEnd payload names the id asked for, with the transcript path and
// working directory, and the result frame is an error naming it too.
// sr:proves session-resume-unknown/claude
func TestT017_83_UnknownResumeNamesTheIDEverywhere(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"SessionEnd": payloadLogger(t, dir, "log.sh", log, "")})
	const asked = "00000000-0000-4000-8000-0000000000ff"
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--resume", asked, "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	assert.Equal(t, 1, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 1)
	assert.Equal(t, asked, ps[0]["session_id"])
	assert.Equal(t, "other", ps[0]["reason"])
	real, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	assert.Equal(t, real, ps[0]["cwd"], "the working directory")
	assert.Contains(t, ps[0]["transcript_path"], asked+".jsonl")
	var result map[string]any
	for _, l := range strings.Split(out, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["type"] == "result" {
			result = f
		}
	}
	require.NotNil(t, result, out)
	assert.Equal(t, true, result["is_error"])
	assert.Equal(t, asked, result["session_id"])
}

// A resume with A10N_MOCK_NO_RESUME=1 of a session that exists is refused
// exactly as the resume of a session that never existed is, without the env
// var (adr fail-fast: the same message, exit status and result frame, bar the id).
// sr:proves session-resume-unknown/claude
// sr:invariant no-resume
func TestT017_83_NoResumeIsAnUnknownSessionsRefusal(t *testing.T) {
	dir, cfg, _ := sessionSetup(t, "SessionStart")
	run := func(env []string, id string) (string, int) {
		out, code := runInDir(t, dir, env, "--script", script(t, dir, "s2"), "--resume", id,
			"--project-dir", dir, "--config-dir", cfg, "--output-format", "stream-json", "-p", "again")
		return strings.ReplaceAll(out, id, "<ID>"), code
	}
	hidden, hiddenCode := run([]string{"A10N_MOCK_NO_RESUME=1"}, origID)
	unknown, unknownCode := run(nil, "00000000-0000-4000-8000-0000000000ee")
	assert.Equal(t, 1, hiddenCode)
	assert.Equal(t, unknownCode, hiddenCode)
	assert.Equal(t, unknown, hidden, "the same refusal, bar the id")
}
