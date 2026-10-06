package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs of the session's own life: how it starts, how its stream
// opens and closes, what its end hooks print and how they fail. Each is
// replayed on the mock exactly as it was run (hook trust bypassed, as every
// recording did) and compared with what the real harness did.

const trustWarning = "`--dangerously-bypass-hook-trust` is enabled. Enabled hooks may run without review for this invocation."

// replayAsRun is replay with the flag every recording was run with.
func replayAsRun(t *testing.T, rec recording, calls ...string) result {
	t.Helper()
	if calls == nil {
		calls = rec.calls
	}
	_, noJSON := os.Stat(filepath.Join(rec.setup, "no-json"))
	return execMock(t, scenario{
		HooksJSON:   readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:       map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:      callThenResult,
		Prompt:      strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:         withCalls(t, calls...),
		BypassTrust: true,
		NoJSON:      noJSON == nil,
	})
}

func (rec recording) stream(t *testing.T) []map[string]any {
	return jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl")))
}

func (rec recording) payloads(t *testing.T) []map[string]any {
	return jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
}

// transcript is every rollout file of the recorded session, as text.
func (rec recording) transcript(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(rec.sample, "transcript", "*"))
	require.NoError(t, err)
	var all []string
	for _, f := range files {
		all = append(all, readFile(t, f))
	}
	return strings.Join(all, "\n")
}

// eventNames names the hook events of a log, in order (lines that are not a
// payload, like the scenario's own markers, are skipped).
func eventNames(log []map[string]any) []string {
	var out []string
	for _, l := range log {
		if ev, ok := l["hook_event_name"].(string); ok {
			out = append(out, ev)
		}
	}
	return out
}

// marks are the values of one key of the log's marker lines, sorted: the hooks
// of one event run together, so their order is not one.
func marks(log []map[string]any, key string) []string {
	var out []string
	for _, l := range log {
		if v, ok := l[key].(string); ok {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// errorItems are the messages of the stream's error items, in order, with the
// configuration directory masked.
func errorItems(stream []map[string]any, home string) []string {
	var out []string
	for _, e := range stream {
		if item, _ := e["item"].(map[string]any); item["type"] == "error" {
			out = append(out, strings.ReplaceAll(item["message"].(string), home, "<CODEX_HOME>"))
		}
	}
	return out
}

// The stream of a run opens as the recordings' did: the thread, the two notices
// that hook trust is bypassed (item_0, item_1, items of type error), and the
// turn; the first command is item_2. A run that stops at its prompt (a prompt
// hook that refuses) streams only that and a turn.completed whose usage is all
// zero, with no item between (runs/prompt-blocked).
// sr:proves noninteractive-run/codex
func TestStreamOpensAsRecordedAndAPromptStoppedRunHasNoItems(t *testing.T) {
	lead := func(stream []map[string]any) (out []string) {
		for _, e := range stream {
			s := fmt.Sprint(e["type"])
			if item, ok := e["item"].(map[string]any); ok {
				s += fmt.Sprintf(" %v %v %v", item["id"], item["type"], item["message"])
			}
			out = append(out, s)
			if e["type"] == "turn.started" {
				break
			}
		}
		return
	}
	for _, name := range []string{"session-end", "stops", "hook-exit-codes", "prompt-blocked", "session-end-hook-output"} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			got := replayAsRun(t, rec)
			require.Equal(t, 0, got.Code, got.Stderr)
			want := lead(rec.stream(t))
			require.Len(t, want, 4)
			assert.Equal(t, "item.completed item_0 error "+trustWarning, want[1])
			assert.Equal(t, "item.completed item_1 error "+trustWarning, want[2])
			assert.Equal(t, want, lead(got.stream()))
		})
	}

	rec := loadRecording(t, "prompt-blocked")
	got := replayAsRun(t, rec)
	want := rec.stream(t)
	for _, e := range want {
		delete(e, "thread_id")
	}
	require.Len(t, want, 5)
	assert.Equal(t, "turn.completed", want[4]["type"])
	assert.Equal(t, map[string]any{"input_tokens": float64(0), "cached_input_tokens": float64(0),
		"cache_write_input_tokens": float64(0), "output_tokens": float64(0), "reasoning_output_tokens": float64(0)}, want[4]["usage"])
	gs := got.stream()
	for _, e := range gs {
		delete(e, "thread_id")
	}
	assert.Equal(t, want, gs, "the same five events: no command, no message")
	assert.Equal(t, []string{"SessionStart", "UserPromptSubmit"}, eventNames(got.hookLog()), "the refused prompt ends the run before anything else but the end hooks")
}

// When end-of-turn hooks keep the turn going, the stream carries one
// agent_message per turn of the model, then a single turn.completed that ends
// it: in the recordings the messages after the last command are as many as the
// Stop hooks that ran (runs/stops: 3, runs/hook-exit-codes: 2), and the run's
// plain output is the last of them only.
// sr:proves noninteractive-run/codex
func TestSeveralAgentMessagesBeforeTheOneTurnCompleted(t *testing.T) {
	finalMessages := func(stream []map[string]any) (msgs []string, completed int) {
		for _, e := range stream {
			item, _ := e["item"].(map[string]any)
			switch {
			case e["type"] == "turn.completed":
				completed++
			case item["type"] == "command_execution" && e["type"] == "item.completed":
				msgs = nil
			case item["type"] == "agent_message":
				msgs = append(msgs, item["text"].(string))
			}
		}
		return
	}
	for name, stops := range map[string]int{"stops": 3, "hook-exit-codes": 2} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			recMsgs, recDone := finalMessages(rec.stream(t))
			recStops := 0
			for _, ev := range eventNames(rec.payloads(t)) {
				if ev == "Stop" {
					recStops++
				}
			}
			assert.Equal(t, stops, recStops)
			assert.Len(t, recMsgs, stops, "recorded: one message per Stop hook that ran, after the last command")
			assert.Equal(t, 1, recDone)
			last := rec.stream(t)
			assert.Equal(t, "turn.completed", last[len(last)-1]["type"])

			got := replayAsRun(t, rec)
			require.Equal(t, 0, got.Code, got.Stderr)
			gotMsgs, gotDone := finalMessages(got.stream())
			assert.Equal(t, 1, gotDone)
			assert.Len(t, gotMsgs, stops)
			gs := got.stream()
			assert.Equal(t, "turn.completed", gs[len(gs)-1]["type"], "the one end is the last event")
		})
	}

	once := `cat >/dev/null; if [ ! -f "$TMPDIR/once" ]; then : >"$TMPDIR/once"; echo again >&2; exit 2; fi`
	r := execMock(t, scenario{
		NoJSON:    true,
		HooksJSON: hooksJSON("sh hook.sh", "Stop"),
		Files:     map[string]string{"hook.sh": once},
		Script:    callThenResult, Prompt: "go", Env: withCalls(t),
	})
	require.Equal(t, 0, r.Code, r.Stderr)
	assert.Equal(t, "DONE\n", r.Stdout, "two turns of the model, one message on stdout")
	assert.Equal(t, 2, strings.Count(r.rollout(t), `"assistant"`))
}

// Without --json a run prints its progress on stderr and only the final agent
// message on stdout, as the recording did (runs/noninteractive-run-text-output):
// a banner (release, directory, model, session), the prompt, the notices, each
// command with how it ended and what it printed, the agent's message, the
// tokens used; nothing a hook printed reaches stdout.
// sr:proves noninteractive-run/codex
func TestProgressOnStderrAndOnlyTheFinalMessageOnStdout(t *testing.T) {
	rec := loadRecording(t, "noninteractive-run-text-output")
	assert.Equal(t, "DONE\n", readFile(t, filepath.Join(rec.sample, "stream.jsonl")), "recorded stdout")
	want := progressShape(readFile(t, filepath.Join(rec.sample, "stderr.txt")))
	require.Contains(t, want, "exited 3 in Nms:")

	got := replayAsRun(t, rec, "echo hi", "sh -c 'echo oops; exit 3'")
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, "DONE\n", got.Stdout)
	assert.Equal(t, want, progressShape(got.Stderr))
	assert.Contains(t, got.Stderr, "workdir: "+got.Repo)
	assert.Contains(t, got.Stderr, "session id: ")
	for _, s := range []string{"HOOK-OUT-START", "HOOK-OUT-END"} {
		assert.NotContains(t, got.Stdout, s)
	}
	assert.Contains(t, got.hookLog()[0]["hook_event_name"], "SessionStart", "the hooks ran")
}

var (
	tokens   = regexp.MustCompile(`(?m)^tokens used\n[\d,]+$`)
	duration = regexp.MustCompile(`in \d+ms:`)
	command  = regexp.MustCompile(`(?m)^/bin/\S+ .* in \S+$`)
	banner   = regexp.MustCompile(`(?m)^(workdir|model|session id): .*$`)
	// what the mock does not model of the progress: the stdin notice, the
	// configuration lines of the banner and a line per hook run
	unmodeled = regexp.MustCompile(`(?m)^(Reading additional input from stdin\.\.\.|provider: .*|approval: .*|sandbox: .*|reasoning .*|hook: .*)\n`)
)

// progressShape is the progress with what a run varies masked (ids, paths,
// durations, token counts, the shell's own quoting of a command) and what the
// mock does not model removed.
func progressShape(stderr string) string {
	s := unmodeled.ReplaceAllString(stderr, "")
	s = banner.ReplaceAllString(s, "$1: <X>")
	s = command.ReplaceAllString(s, "<COMMAND>")
	s = duration.ReplaceAllString(s, "in Nms:")
	return tokens.ReplaceAllString(s, "tokens used\n<N>")
}

// What a SessionEnd hook prints is not recorded anywhere: three handlers
// printed plain text, JSON additionalContext and a JSON systemMessage, and none
// of it is in the transcript, the event stream or the notices on stderr
// (runs/session-end-hook-output); the mock keeps none of it either.
// sr:proves session-end-hook/codex
func TestWhatARealSessionEndHookPrintsIsNotRecorded(t *testing.T) {
	rec := loadRecording(t, "session-end-hook-output")
	got := replayAsRun(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	secrets := []string{"SE-PLAIN-OUT", "SE-CONTEXT-OUT", "SE-MESSAGE-OUT"}
	recorded := []string{rec.transcript(t), readFile(t, filepath.Join(rec.sample, "stream.jsonl")), readFile(t, filepath.Join(rec.sample, "stderr.txt"))}
	mock := []string{got.rollout(t), got.Stdout, got.Stderr}
	for _, s := range secrets {
		for _, where := range append(recorded, mock...) {
			assert.NotContains(t, where, s)
		}
	}
	assert.NotEmpty(t, rec.transcript(t))
	assert.Equal(t, []string{"context", "message", "plain"}, marks(rec.payloads(t), "ran"), "recorded: all three printed")
	assert.Equal(t, marks(rec.payloads(t), "ran"), marks(got.hookLog(), "ran"), "and so did the mock's")
	assert.Equal(t, []string{"SessionEnd", "SessionEnd", "SessionEnd"}, eventNames(got.hookLog()))
	assert.Empty(t, developerTexts(t, got.rollout(t)))
}

// SessionEnd hooks that fail do not end the run abnormally, and a failure is
// not reported where the run's output shows it: of four handlers (exit 1, exit
// 2, a 1-second timeout over a 5-second sleep, and one marked async that takes
// half a second) every one ran, the timed-out one did not finish, the async one
// did finish before the run ended (it ran synchronously, with a notice that
// says so, naming the hooks file), the run exited 0 with its turn completed,
// and neither the stream, stderr nor the transcript mention a failure
// (runs/session-end-hook-failure).
// sr:proves session-end-hook/codex
// sr:proves hook-timeout/codex
func TestSessionEndHooksThatFailOrAreAsyncAsRecorded(t *testing.T) {
	rec := loadRecording(t, "session-end-hook-failure")
	assert.Equal(t, "0\n", readFile(t, filepath.Join(rec.sample, "exit.txt")))
	got := replayAsRun(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)

	assert.Equal(t, []string{"async", "exit1", "exit2", "slow"}, marks(rec.payloads(t), "ran"))
	assert.Equal(t, marks(rec.payloads(t), "ran"), marks(got.hookLog(), "ran"), "every handler ran")
	assert.Equal(t, []string{"async"}, marks(rec.payloads(t), "done"), "recorded: the async one finished, the timed-out one did not")
	assert.Equal(t, marks(rec.payloads(t), "done"), marks(got.hookLog(), "done"))

	asyncWarning := "running async SessionEnd hook synchronously in <CODEX_HOME>/hooks.json"
	want := []string{trustWarning, trustWarning, asyncWarning}
	assert.Equal(t, want, errorItems(rec.stream(t), "<TMP>/home/.codex"), "recorded notices: no failure among them")
	assert.Equal(t, want, errorItems(got.stream(), got.Home))
	for _, s := range []string{"SE-EXIT1-STDERR", "SE-EXIT2-STDERR", "timed out", "failed"} {
		for _, where := range []string{readFile(t, filepath.Join(rec.sample, "stderr.txt")), rec.transcript(t), got.Stderr, got.rollout(t)} {
			assert.NotContains(t, where, s)
		}
	}
	shape := streamShape(got.stream())
	assert.Equal(t, "turn.completed", shape[len(shape)-1])
}

// A session-start hook runs once, first, with the source "startup" for a fresh
// session, before the prompt reaches any hook (runs/stops, runs/hook-exit-codes,
// runs/session-end, each on the real harness and
// replayed); one that exits 2 does not stop the session (runs/hook-exit-codes:
// the prompt hook and the tool calls follow).
// sr:proves session-start-hook/codex
func TestSessionStartFiresOnceFirstWithStartup(t *testing.T) {
	for _, name := range []string{"stops", "hook-exit-codes", "session-end"} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			got := replayAsRun(t, rec)
			require.Equal(t, 0, got.Code, got.Stderr)
			for who, log := range map[string][]map[string]any{"recording": rec.payloads(t), "mock": got.hookLog()} {
				evs := eventNames(log)
				require.GreaterOrEqual(t, len(evs), 2, who)
				assert.Equal(t, "SessionStart", evs[0], who)
				assert.Equal(t, "UserPromptSubmit", evs[1], "%s: the one start precedes the prompt hook", who)
				var starts []map[string]any
				for _, l := range log {
					if l["hook_event_name"] == "SessionStart" {
						starts = append(starts, l)
					}
				}
				require.Len(t, starts, 1, who)
				assert.Equal(t, "startup", starts[0]["source"], who)
				assert.NotEmpty(t, starts[0]["session_id"], who)
			}
		})
	}
}

// The matcher of a session-start hook is applied to the source (hooks#sessionstart):
// on the real harness a hook matching "startup" ran and one matching "resume"
// did not (runs/hook-matchers), and the same on the mock for a fresh session,
// for every way the docs write a matcher that matches startup (omitted, "",
// "*", "startup", the alternation of all four sources, an anchored regular
// expression) and every one that matches only another source.
// sr:proves session-start-hook/codex
func TestSessionStartMatcherIsAppliedToTheSource(t *testing.T) {
	rec := loadRecording(t, "hook-matchers")
	recRan := marks(rec.payloads(t), "ran")
	assert.Contains(t, recRan, "session-startup")
	assert.NotContains(t, recRan, "session-resume")
	got := replayAsRun(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Contains(t, marks(got.hookLog(), "ran"), "session-startup")
	assert.NotContains(t, marks(got.hookLog(), "ran"), "session-resume")

	for matcher, runs := range map[string]bool{
		"": true, "*": true, "startup": true, "startup|resume|clear|compact": true, "^startup$": true, "start.*": true,
		"resume": false, "clear": false, "compact": false, "^compact$": false, "resume|clear|compact": false,
	} {
		t.Run("matcher="+matcher, func(t *testing.T) {
			hooks := fmt.Sprintf(`{"hooks":{"SessionStart":[{"matcher":%q,"hooks":[{"type":"command","command":"sh hook.sh"}]}]}}`, matcher)
			r := execMock(t, scenario{
				HooksJSON: hooks,
				Files:     map[string]string{"hook.sh": logHook},
				Script:    callThenResult, Prompt: "go", Env: withCalls(t),
			})
			require.Equal(t, 0, r.Code, r.Stderr)
			assert.Equal(t, runs, len(eventsOf(r, "SessionStart")) == 1, "matcher %q", matcher)
		})
	}
}

// A session-start hook that says continue:false does not stop the session from
// starting: the thread, its transcript and the hook's own run exist; what it
// ends is the turn, with no prompt hook, no tool call, no message and no model
// request (usage all zero), as the real harness did (runs/session-start-continue-false).
// sr:proves session-start-hook/codex
func TestSessionStartContinueFalseEndsTheTurnNotTheSession(t *testing.T) {
	rec := loadRecording(t, "session-start-continue-false")
	assert.Equal(t, []string{"SessionStart"}, eventNames(rec.payloads(t)))
	assert.Contains(t, rec.transcript(t), `"type":"session_meta"`)
	want := rec.stream(t)

	got := replayAsRun(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, []string{"SessionStart"}, eventNames(got.hookLog()))
	assert.Equal(t, "startup", eventsOf(got, "SessionStart")[0]["source"])
	gs := got.stream()
	require.Len(t, gs, len(want))
	for i := range gs {
		delete(gs[i], "thread_id")
		delete(want[i], "thread_id")
	}
	assert.Equal(t, want, gs)
	assert.Contains(t, got.rollout(t), `"session_meta"`, "the session's transcript exists")
	cmds, _ := got.commands()
	assert.Empty(t, cmds)
}
