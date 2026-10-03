package e2e

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopScript is the agent of the stop recordings: it makes the calls the model
// made, then answers DONE, and DONE2 once the turn was continued by a hook's
// reason (the prompt every recorded scenario gives it).
const stopScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
cmd=$(sed -n "$((n+1))p" "$CALLS")
if [ -n "$cmd" ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_%s","name":"Bash","input":{"command":%s}}]}}\n' "$n" "$(printf '%s' "$cmd" | jq -Rs .)"
  exit 0
fi
if [ "$(grep -c hook_prompt "$A10N_MOCK_SESSION_FILE")" = 0 ]; then say=DONE; else say=DONE2; fi
printf '{"type":"assistant","message":{"content":[{"type":"text","text":"%s"}]}}\n{"type":"result","subtype":"success","result":"%s"}\n' "$say" "$say"
`

// emptyReplyScript makes the calls the model made, then replies with no text.
const emptyReplyScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
cmd=$(sed -n "$((n+1))p" "$CALLS")
if [ -n "$cmd" ]; then
  printf '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_%s","name":"Bash","input":{"command":%s}}]}}\n' "$n" "$(printf '%s' "$cmd" | jq -Rs .)"
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":""}]}}' '{"type":"result","subtype":"success","result":""}'
`

var hookPromptText = regexp.MustCompile(`(?s)^<hook_prompt hook_run_id="[^"]*">(.*)</hook_prompt>$`)

// hookPrompts are the texts of the user-role messages of a rollout that wrap a
// hook's reason in a hook_prompt element, in order.
func hookPrompts(t *testing.T, rollout string) []string {
	t.Helper()
	var out []string
	for _, l := range jsonLines(rollout) {
		p, _ := l["payload"].(map[string]any)
		if l["type"] != "response_item" || p["type"] != "message" || p["role"] != "user" {
			continue
		}
		parts, _ := p["content"].([]any)
		for _, c := range parts {
			text, _ := c.(map[string]any)["text"].(string)
			if m := hookPromptText.FindStringSubmatch(text); m != nil {
				out = append(out, m[1])
			}
		}
	}
	return out
}

// replayStops replays a recorded run with the stop agent.
func replayStops(t *testing.T, rec recording) result {
	t.Helper()
	return execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    stopScript,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       withCalls(t, rec.calls...),
	})
}

func stopPayloads(lines []map[string]any) []map[string]any {
	var out []map[string]any
	for _, l := range lines {
		if l["hook_event_name"] == "Stop" {
			out = append(out, l)
		}
	}
	return out
}

func field(stops []map[string]any, key string) []any {
	var out []any
	for _, s := range stops {
		out = append(out, s[key])
	}
	return out
}

// agentTexts are the messages of the agent in an event stream, in order.
func agentTexts(events []map[string]any) []string {
	var out []string
	for _, e := range events {
		item, _ := e["item"].(map[string]any)
		if e["type"] == "item.completed" && item["type"] == "agent_message" {
			out = append(out, item["text"].(string))
		}
	}
	return out
}

func countType(events []map[string]any, typ string) int {
	n := 0
	for _, e := range events {
		if e["type"] == typ {
			n++
		}
	}
	return n
}

// When a stop hook blocks, by a block decision or by exiting 2 with the reason
// on stderr, the reason is handed back to the agent as a user-role hook_prompt
// message, the agent goes on (a message of its own after each block), the
// payload of the next Stop says the turn is already continuing, and the caller
// sees the turn start and complete once. Replayed from the recorded runs: runs/stops
// (a block decision, then exit 2, then the turn is let end: three Stops) and
// runs/hook-exit-codes (one exit 2 with its stderr reason: two Stops).
// sr:proves stop-block-continuation/codex
func TestStopBlockHandsItsReasonBackAndTheTurnGoesOn(t *testing.T) {
	for _, tc := range []struct {
		run     string
		reasons []string
		active  []any
	}{
		{"stops", []string{"BLOCK-JSON-REASON", "BLOCK-EXIT2-REASON"}, []any{false, true, true}},
		{"hook-exit-codes", []string{"Before finishing, reply with the single word STOPPED-ONCE."}, []any{false, true}},
	} {
		t.Run(tc.run, func(t *testing.T) {
			rec := loadRecording(t, tc.run)
			recStream := jsonLines(readFile(t, filepath.Join(rec.sample, "stream.jsonl")))
			recStops := stopPayloads(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
			require.Equal(t, tc.active, field(recStops, "stop_hook_active"), "the recording")
			require.Equal(t, tc.reasons, hookPrompts(t, recordedRollout(t, rec)), "the recording hands the reasons back")

			got := replayStops(t, rec)
			require.Equal(t, 0, got.Code, got.Stderr)
			assert.Equal(t, tc.reasons, hookPrompts(t, got.rollout(t)), "each reason, in order, as a user message")
			assert.Equal(t, tc.active, field(stopPayloads(got.hookLog()), "stop_hook_active"), "not continuing, then continuing")

			// the agent goes on after each block: one message per Stop, after its last tool call
			tail := func(events []map[string]any) []string {
				msgs := agentTexts(events)
				return msgs[len(msgs)-len(tc.active):]
			}
			assert.Len(t, tail(got.stream()), len(tc.active))
			if tc.run == "stops" {
				assert.Equal(t, []string{"DONE", "DONE2", "DONE2"}, tail(recStream))
				assert.Equal(t, tail(recStream), tail(got.stream()))
			}

			// one end for the caller
			for _, events := range [][]map[string]any{recStream, got.stream()} {
				assert.Equal(t, 1, countType(events, "turn.started"))
				assert.Equal(t, 1, countType(events, "turn.completed"))
				assert.Equal(t, "turn.completed", events[len(events)-1]["type"])
			}
		})
	}
}

// A Stop hook answering continue:false ends the turn, whatever the other Stop
// hooks decided: here a second hook blocks (and is consulted) yet nothing is
// handed back, there is no second Stop, and the turn ends once
// (runs/stop-continue-false; hooks#stop: continue:false takes precedence over
// continuation decisions from other matching Stop hooks).
// sr:proves stop-block-continuation/codex
func TestStopContinueFalseTakesPrecedenceOverABlock(t *testing.T) {
	rec := loadRecording(t, "stop-continue-false")
	recLog := jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	require.Len(t, stopPayloads(recLog), 2, "the recording: both hooks saw the one Stop")
	require.Empty(t, hookPrompts(t, recordedRollout(t, rec)))

	got := replayStops(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	log := got.hookLog()
	assert.Len(t, stopPayloads(log), 2, "both hooks ran for the one Stop, and no later one")
	assert.Equal(t, []any{false, false}, field(stopPayloads(log), "stop_hook_active"))
	var consulted []any
	for _, l := range log {
		if r, ok := l["hook_result"].(map[string]any); ok && r["hook"] == "block" {
			consulted = append(consulted, r["call"])
		}
	}
	assert.Equal(t, []any{float64(1)}, consulted, "the blocking hook was consulted once")
	assert.Empty(t, hookPrompts(t, got.rollout(t)), "its reason was not handed back")
	assert.Equal(t, []string{"DONE"}, agentTexts(got.stream()))
	assert.Equal(t, 1, countType(got.stream(), "turn.completed"))
}

// The payload of every Stop carries the agent's latest message and whether the
// turn was already continued by a Stop hook, together with the session, the
// turn (the same one across the Stops of a turn), the transcript, the working
// directory, the model and the permission mode; no field lists background
// tasks, though one was running in the recorded run (runs/stops: three Stops,
// DONE then DONE2 twice; runs/bg-bash-reaped-at-exit; a reply without text is a
// null message: runs/stop-no-message).
// sr:proves stop-hook-payload/codex
func TestStopPayloadCarriesTheLatestMessageAndWhetherContinuing(t *testing.T) {
	rec := loadRecording(t, "stops")
	want := stopPayloads(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Len(t, want, 3)
	require.Equal(t, []any{"DONE", "DONE2", "DONE2"}, field(want, "last_assistant_message"))
	require.Equal(t, []any{false, true, true}, field(want, "stop_hook_active"))

	run := replayStops(t, rec)
	got := stopPayloads(run.hookLog())
	require.Len(t, got, 3)
	wantRepo, err := filepath.EvalSymlinks(run.Repo)
	require.NoError(t, err)
	sid := sessionIDOf(t, run)
	assert.Equal(t, field(want, "last_assistant_message"), field(got, "last_assistant_message"), "the new message on each continued Stop")
	assert.Equal(t, field(want, "stop_hook_active"), field(got, "stop_hook_active"))
	for i := range got {
		assert.Equal(t, keysOf(want[i]), keysOf(got[i]), "the fields of Stop %d", i+1)
		assert.Equal(t, "Stop", got[i]["hook_event_name"])
		assert.Equal(t, got[0]["turn_id"], got[i]["turn_id"], "one turn id across the Stops of a turn")
		assert.Equal(t, got[0]["session_id"], got[i]["session_id"])
		assert.NotEmpty(t, got[i]["turn_id"])
		assert.Equal(t, sid, got[i]["session_id"], "the session's id")
		assert.Equal(t, wantRepo, got[i]["cwd"])
		tp, _ := got[i]["transcript_path"].(string)
		assert.True(t, strings.HasPrefix(tp, run.Home) && strings.Contains(tp, sid), "transcript_path %q names the session's file", tp)
		assert.Equal(t, "mock-model", got[i]["model"], "the model the run was given")
		assert.Equal(t, want[i]["permission_mode"], got[i]["permission_mode"])
	}

	// a reply with no text is a null message (runs/stop-no-message: the stream
	// shows an agent message of "", the payload null)
	noMsg := loadRecording(t, "stop-no-message")
	nullStop := stopPayloads(jsonLines(readFile(t, filepath.Join(noMsg.sample, "payloads.jsonl"))))
	require.Len(t, nullStop, 1)
	require.Contains(t, nullStop[0], "last_assistant_message")
	require.Nil(t, nullStop[0]["last_assistant_message"])
	require.Equal(t, []string{""}, agentTexts(jsonLines(readFile(t, filepath.Join(noMsg.sample, "stream.jsonl")))))
	empty := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(noMsg.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(noMsg.setup, "hook.sh"))},
		Script:    emptyReplyScript,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(noMsg.setup, "prompt.txt"))),
		Env:       withCalls(t, noMsg.calls...),
	})
	require.Equal(t, 0, empty.Code, empty.Stderr)
	gotNull := stopPayloads(empty.hookLog())
	require.Len(t, gotNull, 1)
	assert.Contains(t, gotNull[0], "last_assistant_message")
	assert.Nil(t, gotNull[0]["last_assistant_message"])
	assert.Equal(t, keysOf(nullStop[0]), keysOf(gotNull[0]))

	// a background command still running at the Stop adds no field: the mock,
	// driven into the recorded situation (a command that outlives the call, the
	// turn ended at once with LAUNCHED), sends the recorded Stop's fields
	bgRec := loadRecording(t, "bg-bash-reaped-at-exit")
	bg := stopPayloads(jsonLines(readFile(t, filepath.Join(bgRec.sample, "payloads.jsonl"))))
	require.Len(t, bg, 1)
	assert.Equal(t, keysOf(want[0]), keysOf(bg[0]), "the recorded Stop with a command still running")
	assert.Equal(t, "LAUNCHED", bg[0]["last_assistant_message"])
	prompt := strings.TrimSpace(readFile(t, filepath.Join(bgRec.setup, "prompt.txt")))
	running := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(bgRec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(bgRec.setup, "hook.sh"))},
		Script:    backgroundScript,
		Prompt:    prompt,
		Env:       withCalls(t, prompt[strings.Index(prompt, "The command: ")+len("The command: "):]),
	})
	require.Equal(t, 0, running.Code, running.Stderr)
	gotBg := stopPayloads(running.hookLog())
	require.Len(t, gotBg, 1)
	assert.Equal(t, keysOf(bg[0]), keysOf(gotBg[0]))
	assert.Equal(t, "LAUNCHED", gotBg[0]["last_assistant_message"])
	assert.Equal(t, false, gotBg[0]["stop_hook_active"])
	for _, p := range [][]map[string]any{got, gotBg} {
		raw, err := json.Marshal(p[0])
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "background")
	}
}
