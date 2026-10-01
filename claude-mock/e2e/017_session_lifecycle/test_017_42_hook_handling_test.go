package e2e

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// groupHook is a hook that logs "<group> <event> <tool>" for the group named by
// its argument, as snapshots/runs/matcher's hook does.
func groupHook(t *testing.T, dir, log string) string {
	t.Helper()
	return write(t, filepath.Join(dir, "group.sh"), `#!/bin/sh
IN=$(cat)
EV=$(printf '%s' "$IN" | sed -n 's/.*"hook_event_name":"\([A-Za-z]*\)".*/\1/p')
TOOL=$(printf '%s' "$IN" | sed -n 's/.*"tool_name":"\([A-Za-z]*\)".*/\1/p')
echo "$1 $EV $TOOL" >> `+log+`
`, 0o755)
}

func groupSettings(t *testing.T, dir, hook string, groups map[string][][2]string) {
	t.Helper()
	var ev []string
	for event, gs := range groups {
		var entries []string
		for _, g := range gs {
			m := ""
			if g[0] != "-" {
				m = `"matcher":"` + g[0] + `",`
			}
			entries = append(entries, `{`+m+`"hooks":[{"type":"command","command":"`+hook+` `+g[1]+`"}]}`)
		}
		ev = append(ev, `"`+event+`":[`+strings.Join(entries, ",")+`]`)
	}
	write(t, filepath.Join(dir, ".claude", "settings.json"), `{"hooks":{`+strings.Join(ev, ",")+`}}`, 0o644)
}

func loggedGroups(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	sort.Strings(lines)
	return lines
}

// TestT017_42_MatcherSelectsBySubject: a matcher of plain names is an exact
// name or a list of them, any other matcher is a regular expression searched
// in the subject, and an empty, "*" or missing matcher selects everything
// (docs, Matcher patterns; snapshots/runs/matcher: Bash and Bash|Read select
// their tools, Rea and read select none, ^Rea.* selects Read, and a SessionStart
// matcher selects by how the session started).
// sr:proves hook-matcher-filter/claude
func TestT017_42_MatcherSelectsBySubject(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "groups.log")
	h := groupHook(t, dir, log)
	groupSettings(t, dir, h, map[string][][2]string{
		"PreToolUse": {{"Bash", "exact-bash"}, {"Edit|Write", "exact-list-edit-write"}, {"Bash|Read", "exact-list-bash-read"},
			{"Rea", "exact-rea-prefix"}, {`^Rea.*`, "regex-read"}, {"read", "exact-lowercase-read"}, {"Edit, Read", "exact-list-spaced"}},
		"PostToolUse":  {{"Read", "post-exact-read"}, {"*", "post-all"}, {"-", "post-no-matcher"}},
		"SessionStart": {{"resume", "start-resume"}, {"startup", "start-startup"}},
	})
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`), toolUse("r1", "Read", `{"file_path":"`+h+`"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "mt-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, []string{
		"exact-bash PreToolUse Bash", "exact-list-bash-read PreToolUse Bash", "exact-list-bash-read PreToolUse Read",
		"exact-list-spaced PreToolUse Read", "post-all PostToolUse Bash", "post-all PostToolUse Read",
		"post-exact-read PostToolUse Read", "post-no-matcher PostToolUse Bash", "post-no-matcher PostToolUse Read",
		"regex-read PreToolUse Read", "start-startup SessionStart",
	}, loggedGroups(t, log))
}

// TestT017_43_EventsFilterOnTheirOwnSubject: each event's matcher filters on
// its own subject (docs, Matcher patterns): a session's end on its reason, a
// sub-agent's start and stop on its type, a compaction on its trigger. An event
// with no subject takes every hook, whatever matcher it was given.
// sr:proves hook-matcher-filter/claude
func TestT017_43_EventsFilterOnTheirOwnSubject(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "groups.log")
	h := groupHook(t, dir, log)
	groupSettings(t, dir, h, map[string][][2]string{
		"SessionEnd":       {{"clear", "end-clear"}, {"other", "end-other"}},
		"SubagentStart":    {{"general-purpose", "sub-gp"}, {"Explore", "sub-explore"}},
		"SubagentStop":     {{"Explore|general-purpose", "substop-list"}, {"Plan", "substop-plan"}},
		"PreCompact":       {{"manual", "pre-manual"}, {"auto", "pre-auto"}},
		"UserPromptSubmit": {{"NoSuchThing", "prompt-ignored-matcher"}},
		"Stop":             {{"NoSuchThing", "stop-ignored-matcher"}},
	})
	sub := script(t, dir, "sub")
	orch := write(t, filepath.Join(dir, "orch.sh"), `#!/bin/sh
F="$A10N_MOCK_SESSION_FILE"
if ! grep -q turn-a "$F" 2>/dev/null; then
cat <<'JSONL'
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"ag1turn-a","name":"Agent","input":{"prompt":"p","description":"d","script":"`+sub+`"}}]}}
JSONL
exit 0
fi
if ! grep -q turn-b "$F" 2>/dev/null; then
cat <<'JSONL'
{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"turn-b [context compacted]"}}
JSONL
fi
echo '{"type":"result","subtype":"success","result":"done"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", orch, "--session-id", "es-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, []string{
		"end-other SessionEnd", "pre-auto PreCompact", "prompt-ignored-matcher UserPromptSubmit",
		"stop-ignored-matcher Stop", "sub-gp SubagentStart", "substop-list SubagentStop",
	}, loggedGroups(t, log))
}

// TestT017_44_HookTimeoutKillsTheHookAndWhatItSpawned: a hook that outlives
// its timeout is cancelled: the process group dies (the grandchild never
// writes), the deny it would print late is discarded, so the call goes ahead,
// and the transcript says it was cancelled (snapshots/runs/hook-timeout:
// hook_cancelled with timedOut and timeoutMs). A session-end hook is bounded by
// the harness's 1.5-second budget when it sets no timeout of its own.
// sr:proves hook-timeout/claude
// sr:proves hook-output-transcript-records/claude
func TestT017_44_HookTimeoutKillsTheHookAndWhatItSpawned(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	child := filepath.Join(dir, "grandchild-survived")
	pre := write(t, filepath.Join(dir, "pre.sh"), `#!/bin/sh
cat >/dev/null
( sleep 2; : > `+child+` ) &
sleep 30
echo '{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"LATE-DENY"}}'
`, 0o755)
	end := write(t, filepath.Join(dir, "end.sh"), "#!/bin/sh\ncat >/dev/null\nsleep 30\n", 0o755)
	write(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"`+pre+`","timeout":1}]}],`+
			`"SessionEnd":[{"hooks":[{"type":"command","command":"`+end+`"}]}]}}`, 0o644)
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo AFTER-TIMEOUT; sleep 3"}`))
	start := time.Now()
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "to-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Less(t, time.Since(start), 12*time.Second, "neither the 30-second hook nor the session-end hook may be waited for")
	_, err := os.Stat(child)
	assert.True(t, os.IsNotExist(err), "the hook's grandchild outlived its timeout")

	recs := readRecs(t, transcriptPath(t, cfg, dir, "to-1"))
	var cancelled map[string]any
	for _, r := range recs {
		if r.Type == "attachment" && r.Attachment["type"] == "hook_cancelled" {
			cancelled = r.Attachment
		}
		assert.NotContains(t, r.Raw, "LATE-DENY", "a cancelled hook's output is discarded")
	}
	require.NotNil(t, cancelled, "the cancellation is recorded")
	assert.Equal(t, "PreToolUse:Bash", cancelled["hookName"])
	assert.Equal(t, "PreToolUse", cancelled["hookEvent"])
	assert.Equal(t, true, cancelled["timedOut"])
	assert.EqualValues(t, 1000, cancelled["timeoutMs"])
	assert.Equal(t, pre, cancelled["command"])
	block, _ := toolResultOf(t, recs, "b1turn-s-a")
	assert.Equal(t, "AFTER-TIMEOUT", block["content"], "the call went ahead")
}

// TestT017_45_AllMatchingHooksRunAndTheLastBlockToFinishIsActedOn: three hooks
// on one event all run, together, though two of them block; of the blocks the
// one that finishes last is the one the agent is told (snapshots/runs/all-hooks
// and all-hooks-slow-first: the refusal quoted B's message when B finished
// last, A's when A was the slow one).
// sr:proves hooks-all-matching-run/claude
func TestT017_45_AllMatchingHooksRunAndTheLastBlockToFinishIsActedOn(t *testing.T) {
	for _, tc := range []struct{ name, slow, want string }{
		{"B last", "B", "BLOCK-B"}, {"A last", "A", "BLOCK-A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			log := filepath.Join(dir, "ran.log")
			h := write(t, filepath.Join(dir, "h.sh"), `#!/bin/sh
cat >/dev/null
[ "$1" = `+tc.slow+` ] && sleep 0.6
echo "ran $1" >> `+log+`
case "$1" in A) echo BLOCK-A >&2; exit 2;; B) echo BLOCK-B >&2; exit 2;; esac
`, 0o755)
			groupSettings(t, dir, h, map[string][][2]string{"PreToolUse": {{"Bash", "A"}, {"Bash", "B"}, {"Bash", "C"}}})
			sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo SHOULD-NOT-RUN"}`))
			start := time.Now()
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "am-1",
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)
			assert.Less(t, time.Since(start), 5*time.Second)
			ran := loggedGroups(t, log)
			assert.Equal(t, []string{"ran A", "ran B", "ran C"}, ran, "every matching hook ran, though two blocked")
			block, _ := toolResultOf(t, readRecs(t, transcriptPath(t, cfg, dir, "am-1")), "b1turn-s-a")
			assert.Equal(t, true, block["is_error"])
			assert.Contains(t, block["content"], tc.want+"\n")
			assert.NotContains(t, block["content"], "SHOULD-NOT-RUN")
		})
	}
}

// TestT017_46_HooksRunTogether: the hooks of one event are started together,
// not one after another (docs: "All matching hooks run in parallel").
// sr:proves hooks-all-matching-run/claude
func TestT017_46_HooksRunTogether(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	slow := write(t, filepath.Join(dir, "slow.sh"), "#!/bin/sh\ncat >/dev/null\nsleep 1\n", 0o755)
	write(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"`+slow+`"},{"type":"command","command":"`+slow+`"},{"type":"command","command":"`+slow+`"}]}]}}`, 0o644)
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`))
	start := time.Now()
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "pl-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Less(t, time.Since(start), 2500*time.Millisecond, "three one-second hooks took as long as one after another")
}
