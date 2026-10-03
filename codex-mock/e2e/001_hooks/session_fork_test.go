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

// The recorded run runs/session-fork: a session that runs `echo ORIGINAL` and
// answers OK; then `codex exec fork <its id>` asked which word that command
// printed (answer ORIGINAL); then `codex exec resume <its id>`.

// execFork runs `exec fork <from> <prompt>` in the repository and CODEX_HOME of r.
func execFork(t *testing.T, r result, from, prompt string) result {
	t.Helper()
	script := filepath.Join(t.TempDir(), "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(callThenResult), 0o755))
	cmd := exec.Command(mockBinary, "exec", "fork", "--json", "--skip-git-repo-check", "--script", script, "-m", "mock-model", from, prompt)
	cmd.Dir = r.Repo
	cmd.Env = append(append(os.Environ(), withCalls(t)...), "CODEX_HOME="+r.Home, "TMPDIR="+r.Tmp, "HOOK_LOG="+filepath.Join(r.Tmp, "hook.log"))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), errb.String())
	return result{Repo: r.Repo, Home: r.Home, Tmp: r.Tmp, Stdout: out.String(), Stderr: errb.String()}
}

// threadIDs are the ids of the sessions the event stream starts, in order.
func threadIDs(r result) (ids []string) {
	for _, e := range r.stream() {
		if e["type"] == "thread.started" {
			ids = append(ids, e["thread_id"].(string))
		}
	}
	return
}

// rolloutOf is the text and the meta record of session id's rollout under home.
func rolloutOf(t *testing.T, home, id string) (string, map[string]any) {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+id+".jsonl"))
	require.Len(t, paths, 1, "the rollout of session %s", id)
	text := readFile(t, paths[0])
	for _, l := range jsonLines(text) {
		if l["type"] == "session_meta" {
			return text, l["payload"].(map[string]any)
		}
	}
	require.Fail(t, "no session_meta")
	return "", nil
}

// agentMessages are the agent's messages in the event stream.
func agentMessages(r result) (out []string) {
	for _, e := range r.stream() {
		if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "agent_message" {
			out = append(out, item["text"].(string))
		}
	}
	return
}

// hooksOf are the hook events a session's hooks saw, as "<event>" or
// "<event>/<source>", from log lines.
func hooksOf(lines []map[string]any, sid string) (out []string) {
	for _, l := range lines {
		if l["session_id"] != sid {
			continue
		}
		name := l["hook_event_name"].(string)
		if src, ok := l["source"].(string); ok {
			name += "/" + src
		}
		out = append(out, name)
	}
	return
}

// A fork continues the conversation under a new session id, in a new rollout
// that does not copy the history: its meta record points at the session it was
// forked from and how much of it (forked_from_id, history_base), and it holds
// only what is said after. SessionStart fires in it with source "fork". The
// original is not touched (runs/session-fork).
// sr:proves session-fork/codex
func TestForkContinuesTheConversationUnderANewSessionAndLeavesTheOriginal(t *testing.T) {
	rec := loadRecording(t, "session-fork")
	recStream := result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))}
	recIDs := threadIDs(recStream)
	require.Len(t, recIDs, 3)
	require.NotEqual(t, recIDs[0], recIDs[1], "the recorded fork has an id of its own")
	require.Equal(t, recIDs[0], recIDs[2], "the recorded resume continues the original")
	wantReplies := agentMessages(recStream)
	require.Equal(t, []string{"OK", "ORIGINAL", "RESUMED"}, wantReplies)
	forkPrompt := strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "then-01-prompt.txt")))

	// what the recording shows of the fork's hooks and its rollout
	recHooks := jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	wantHooks := hooksOf(recHooks, recIDs[1])
	require.Equal(t, []string{"SessionStart/fork", "UserPromptSubmit", "Stop"}, wantHooks)
	recRollouts := map[string]string{}
	recMeta := map[string]map[string]any{}
	files, _ := filepath.Glob(filepath.Join(rec.sample, "transcript", "rollout-*.jsonl"))
	for _, f := range files {
		for _, id := range recIDs[:2] {
			if strings.HasSuffix(f, id+".jsonl") {
				recRollouts[id] = readFile(t, f)
				recMeta[id] = jsonLines(recRollouts[id])[0]["payload"].(map[string]any)
			}
		}
	}
	require.Len(t, recRollouts, 2)
	assert.Equal(t, recIDs[0], recMeta[recIDs[1]]["forked_from_id"])
	assert.Equal(t, recIDs[0], recMeta[recIDs[1]]["history_base"].(map[string]any)["thread_id"])
	recBase := recMeta[recIDs[1]]["history_base"].(map[string]any)
	assert.Positive(t, recBase["end_ordinal_exclusive"], "the recorded fork names how much of the original it continues")
	assert.Equal(t, recBase["end_ordinal_exclusive"], recMeta[recIDs[1]]["forked_from_ordinal_exclusive"])
	assert.NotContains(t, recRollouts[recIDs[1]], "echo ORIGINAL", "the recorded fork's file does not copy the history")
	assert.NotContains(t, recRollouts[recIDs[0]], forkPrompt, "the recorded original has none of the fork")

	// the mock: the same two sessions
	orig := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       withCalls(t, "echo ORIGINAL"),
	})
	require.Equal(t, 0, orig.Code, orig.Stderr)
	origID := threadIDs(orig)[0]
	before, _ := rolloutOf(t, orig.Home, origID)

	fork := execFork(t, orig, origID, forkPrompt)
	forkID := threadIDs(fork)[0]
	assert.NotEqual(t, origID, forkID)
	assert.Equal(t, []string{"DONE"}, agentMessages(fork), "the fork runs its own turn")
	assert.Equal(t, wantHooks, hooksOf(orig.hookLog(), forkID))

	text, meta := rolloutOf(t, fork.Home, forkID)
	assert.Equal(t, origID, meta["forked_from_id"])
	assert.Equal(t, origID, meta["history_base"].(map[string]any)["thread_id"])
	// how much of the original it continues: all of it, as it stood at the fork
	records := float64(len(jsonLines(before)))
	assert.Equal(t, records, meta["history_base"].(map[string]any)["end_ordinal_exclusive"])
	assert.Equal(t, records, meta["forked_from_ordinal_exclusive"])
	assert.NotContains(t, text, "echo ORIGINAL", "the fork's file does not copy the history")
	assert.Contains(t, text, forkPrompt)

	after, _ := rolloutOf(t, orig.Home, origID)
	assert.Equal(t, before, after, "the original is left untouched")
	assert.NotContains(t, after, forkPrompt)
}

// A session started by a fork fires SessionStart with source "fork", under the
// fork's own session id (runs/session-fork).
// sr:proves session-start-hook/codex
func TestForkSessionStartSaysFork(t *testing.T) {
	rec := loadRecording(t, "session-fork")
	var want []string
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if l["hook_event_name"] == "SessionStart" {
			want = append(want, l["source"].(string))
		}
	}
	require.Equal(t, []string{"startup", "fork", "resume"}, want)

	orig := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t),
	})
	origID := threadIDs(orig)[0]
	fork := execFork(t, orig, origID, "again")
	forkID := threadIDs(fork)[0]
	var got []string
	for _, l := range orig.hookLog() {
		if l["hook_event_name"] == "SessionStart" {
			got = append(got, l["source"].(string))
			if l["source"] == "fork" {
				assert.Equal(t, forkID, l["session_id"])
			}
		}
	}
	assert.Equal(t, []string{"startup", "fork"}, got)
}

// A compaction starts the session again: SessionStart fires after it with
// source "compact", as many times as the recording shows (runs/manual-compaction-auto).
// sr:proves session-start-hook/codex
func TestCompactionStartsTheSessionWithSourceCompact(t *testing.T) {
	rec, got := replayCompacting(t, "manual-compaction-auto")
	require.Equal(t, 0, got.Code, got.Stderr)
	sources := func(lines []map[string]any) (out []string) {
		for _, l := range lines {
			if l["hook_event_name"] == "SessionStart" {
				out = append(out, l["source"].(string))
			}
		}
		return
	}
	want := sources(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Equal(t, []string{"startup", "compact", "compact", "compact"}, want)
	assert.Equal(t, want, sources(got.hookLog()))
}
