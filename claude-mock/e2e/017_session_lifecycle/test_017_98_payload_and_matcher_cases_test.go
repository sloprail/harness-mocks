package e2e

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TestT017_100_PromptIDOnTurnEvents: every hook of a turn is told the id of the
// prompt that began it, a UUID that is the same across the turn's events and new
// for the next prompt; SessionStart, which precedes any prompt, has none (the
// hookmix and stops recordings, docs: common input fields).
// sr:proves hook-common-payload/claude
func TestT017_100_PromptIDOnTurnEvents(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"SessionStart": h, "UserPromptSubmit": h, "PreToolUse": h, "PostToolUse": h, "Stop": h})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`))
	for _, args := range [][]string{{"--session-id", "pi-1", "-p", "one"}, {"--resume", "pi-1", "-p", "two"}} {
		out, code := runInDir(t, dir, nil, append([]string{"--script", sc, "--project-dir", dir, "--config-dir", cfg}, args...)...)
		require.Equal(t, 0, code, out)
	}
	ids := map[string][]string{} // event -> prompt ids, in order
	for _, p := range payloads(t, log) {
		ev := p["hook_event_name"].(string)
		id, has := p["prompt_id"].(string)
		if ev == "SessionStart" {
			assert.False(t, has, "SessionStart precedes any prompt")
			continue
		}
		require.True(t, has, "%s carries a prompt_id", ev)
		assert.Regexp(t, uuidRe, id)
		ids[ev] = append(ids[ev], id)
	}
	first := ids["UserPromptSubmit"][0]
	assert.Equal(t, []string{first, first}, []string{ids["PreToolUse"][0], ids["PostToolUse"][0]}, "one id across a turn's events")
	assert.Equal(t, first, ids["Stop"][0])
	second := ids["UserPromptSubmit"][1]
	assert.NotEqual(t, first, second, "the next prompt has its own")
	assert.Equal(t, second, ids["Stop"][1])
}

// TestT017_101_SessionStartMatcherBySource: a SessionStart matcher selects on how the
// session started: startup on a new one, resume on a resumed one, fork on a forked
// one, and a matcher of another source does not fire (docs, SessionStart matcher values).
// sr:proves hook-matcher-filter/claude
func TestT017_101_SessionStartMatcherBySource(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "groups.log")
	h := groupHook(t, dir, log)
	groupSettings(t, dir, h, map[string][][2]string{"SessionStart": {{"startup", "start-startup"}, {"resume", "start-resume"}, {"fork", "start-fork"}}})
	sc := script(t, dir, "s")
	var got [][]string
	for _, args := range [][]string{
		{"--session-id", "sm-1", "-p", "one"},
		{"--resume", "sm-1", "-p", "two"},
		{"--resume", "sm-1", "--fork-session", "--session-id", "sm-2", "-p", "three"},
	} {
		out, code := runInDir(t, dir, nil, append([]string{"--script", sc, "--project-dir", dir, "--config-dir", cfg}, args...)...)
		require.Equal(t, 0, code, out)
		got = append(got, loggedGroups(t, log))
	}
	assert.Equal(t, []string{"start-startup SessionStart"}, got[0])
	assert.Equal(t, []string{"start-resume SessionStart", "start-startup SessionStart"}, got[1], "the resume adds only the resume matcher's")
	assert.Equal(t, []string{"start-fork SessionStart", "start-resume SessionStart", "start-startup SessionStart"}, got[2], "the fork adds only the fork matcher's")
}

// TestT017_102_ForkedAndResumedContext: a SessionStart hook's context is added again
// when a session is forked (its source "fork"); resuming does not run the saved turns'
// PostToolUse and UserPromptSubmit hooks again (their context stays in the transcript,
// where it was saved): only the new prompt's hooks run (docs, Add context for Claude).
// sr:proves hook-additional-context/claude
func TestT017_102_ForkedAndResumedContext(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, `case "$IN" in
*'"hook_event_name":"SessionStart"'*'"source":"fork"'*) echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"FORK-CTX"}}' ;;
esac`)
	settings(t, dir, map[string]string{"SessionStart": h, "UserPromptSubmit": h, "PostToolUse": h})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`))
	run := func(args ...string) {
		out, code := runInDir(t, dir, nil, append([]string{"--script", sc, "--project-dir", dir, "--config-dir", cfg}, args...)...)
		require.Equal(t, 0, code, out)
	}
	run("--session-id", "fc-1", "-p", "one")
	before := len(payloads(t, log))
	run("--resume", "fc-1", "-p", "two")
	var events []string
	for _, p := range payloads(t, log)[before:] {
		events = append(events, p["hook_event_name"].(string))
	}
	assert.Equal(t, []string{"SessionStart", "UserPromptSubmit"}, events, "the saved turn's tool hooks are not run again; the new prompt's script makes no call")
	run("--resume", "fc-1", "--fork-session", "--session-id", "fc-2", "-p", "three")
	got := contextKinds(t, readRecs(t, transcriptPath(t, cfg, dir, "fc-2")))
	require.NotEmpty(t, got)
	assert.Equal(t, [3]string{"", "SessionStart", "FORK-CTX"}, got[len(got)-1], "the fork's start adds its context")
}

// TestT017_103_FailureHookPayloadNamesTheTool: the PostToolUseFailure payload names
// the tool and its input as called: a failing Bash its command, a failing Read its
// file_path as the model gave it (recordings bashfail, tool-errors).
// sr:proves tool-failure-hook/claude
func TestT017_103_FailureHookPayloadNamesTheTool(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PostToolUseFailure": h})
	missing := filepath.Join(dir, "nonexistent-dir", "missing.txt")
	sc := script(t, dir, "s",
		toolUse("b1", "Bash", `{"command":"echo OUT-LINE; echo ERR-LINE >&2; exit 3"}`),
		toolUse("r1", "Read", `{"file_path":"`+missing+`"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "fp-1", "--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	ps := payloads(t, log)
	require.Len(t, ps, 2)
	assert.Equal(t, "Bash", ps[0]["tool_name"])
	assert.Equal(t, map[string]any{"command": "echo OUT-LINE; echo ERR-LINE >&2; exit 3"}, ps[0]["tool_input"])
	assert.Equal(t, "Read", ps[1]["tool_name"])
	assert.Equal(t, map[string]any{"file_path": missing}, ps[1]["tool_input"])
}
