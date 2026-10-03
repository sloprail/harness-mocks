package e2e

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs runs/subagent-lifecycle-hooks (a sub-agent runs one
// command and says SUB-DONE) and runs/subagent-start-refused (the same, its
// start hook exiting 2): the session's hooks on SubagentStart, SubagentStop
// and Stop. The model's spawn_agent call is replayed by a script, the
// sub-agent's work by another.
const (
	spawnThenResult = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_spawn","name":"spawn_agent","input":{"message":"Run the shell command echo SUB-DONE and reply only SUB-DONE.","script":"sub.sh"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"DONE"}]}}' '{"type":"result","subtype":"success","result":"DONE"}'
`
	subScript = `#!/bin/sh
n=$(grep -c function_call_output "$A10N_MOCK_SESSION_FILE")
if [ "$n" = 0 ]; then
  printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"call_sub","name":"Bash","input":{"command":"echo SUB-DONE"}}]}}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"SUB-DONE"}]}}' '{"type":"result","subtype":"success","result":"SUB-DONE"}'
`
)

// replaySubagent runs the mock on a recorded run's setup, the model's calls
// being a spawn_agent whose sub-agent runs echo SUB-DONE.
func replaySubagent(t *testing.T, name string) (rec recording, got result) {
	t.Helper()
	rec = loadRecording(t, name)
	got = execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files: map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh")),
			"sub.sh": subScript},
		Script: spawnThenResult,
		Prompt: strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	return rec, got
}

// byEvent is the hook payloads of a log, by event, in order.
func byEvent(lines []map[string]any) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for _, l := range lines {
		ev, _ := l["hook_event_name"].(string)
		out[ev] = append(out[ev], l)
	}
	return out
}

func keysOf(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Hooks fire when a sub-agent starts and when it stops, and each, with the
// session's Stop, is told what the recording shows: the start names the
// sub-agent (agent_id, agent_type) and its own rollout as transcript_path; the
// stop names the session's transcript_path and, as agent_transcript_path, the
// sub-agent's own, with the sub-agent's last message. A stop payload lists no
// background tasks (runs/subagent-lifecycle-hooks).
// sr:proves subagent-lifecycle-hooks/codex
func TestSubagentStartAndStopHooks(t *testing.T) {
	rec, got := replaySubagent(t, "subagent-lifecycle-hooks")
	want := byEvent(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	have := byEvent(got.hookLog())

	for _, ev := range []string{"SubagentStart", "SubagentStop", "Stop"} {
		require.Len(t, want[ev], 1, ev)
		require.Len(t, have[ev], 1, ev)
		assert.Equal(t, keysOf(want[ev][0]), keysOf(have[ev][0]), ev+" payload fields")
	}
	ws, ps := want["SubagentStart"][0], have["SubagentStart"][0]
	we, pe := want["SubagentStop"][0], have["SubagentStop"][0]
	assert.Equal(t, ws["agent_type"], ps["agent_type"])
	assert.Equal(t, we["stop_hook_active"], pe["stop_hook_active"])
	assert.Equal(t, we["last_assistant_message"], pe["last_assistant_message"])
	assert.Equal(t, "SUB-DONE", pe["last_assistant_message"])
	assert.Equal(t, ps["agent_id"], pe["agent_id"], "the same sub-agent")

	for name, p := range map[string][2]map[string]any{"recorded": {ws, we}, "mock": {ps, pe}} {
		start, stop := p[0], p[1]
		assert.Equal(t, start["transcript_path"], stop["agent_transcript_path"], name+": the sub-agent's own transcript")
		assert.NotEqual(t, stop["transcript_path"], stop["agent_transcript_path"], name+": not the session's")
	}
	assert.Equal(t, want["Stop"][0]["transcript_path"], we["transcript_path"], "recorded: SubagentStop names the session's transcript")
	assert.Equal(t, have["Stop"][0]["transcript_path"], pe["transcript_path"], "SubagentStop names the session's transcript")
	for _, k := range keysOf(pe) {
		assert.NotContains(t, k, "task", "no background tasks are listed")
	}

	// the start hook fires before the stop hook, and both before the session's Stop
	var order []string
	for _, l := range got.hookLog() {
		order = append(order, l["hook_event_name"].(string))
	}
	assert.Equal(t, []string{"SubagentStart", "SubagentStop", "Stop"}, order)

	// the stream shows a spawn and a wait, which ends with the sub-agent's message
	shape := func(r result) (tools []string, said any) {
		for _, e := range r.stream() {
			if item, _ := e["item"].(map[string]any); e["type"] == "item.completed" && item["type"] == "collab_tool_call" {
				tools = append(tools, item["tool"].(string))
				for _, st := range item["agents_states"].(map[string]any) {
					said = st.(map[string]any)["message"]
				}
			}
		}
		return
	}
	wantTools, wantSaid := shape(result{Stdout: readFile(t, filepath.Join(rec.sample, "stream.jsonl"))})
	gotTools, gotSaid := shape(got)
	assert.Equal(t, []string{"spawn_agent", "wait"}, wantTools)
	assert.Equal(t, wantTools, gotTools)
	assert.Equal(t, wantSaid, gotSaid)
}

// The start hook cannot refuse the sub-agent: with it exiting 2 and stating a
// reason, the sub-agent still runs, and still stops (runs/subagent-start-refused).
// sr:proves subagent-lifecycle-hooks/codex
func TestSubagentStartCannotRefuse(t *testing.T) {
	rec, got := replaySubagent(t, "subagent-start-refused")
	want := byEvent(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	have := byEvent(got.hookLog())
	for _, hooks := range []map[string][]map[string]any{want, have} {
		require.Len(t, hooks["SubagentStart"], 1)
		require.Len(t, hooks["SubagentStop"], 1, "the sub-agent ran to its stop")
	}
	assert.Equal(t, "SUB-DONE", have["SubagentStop"][0]["last_assistant_message"])
	assert.Equal(t, want["SubagentStop"][0]["last_assistant_message"], have["SubagentStop"][0]["last_assistant_message"])
	assert.Contains(t, got.Stdout, `"SUB-DONE"`, "the wait reports what the sub-agent said")
}

// A sub-agent hook's matcher is applied to the sub-agent's type, and a start
// hook's continue:false does not stop the sub-agent (hooks#subagentstart).
// sr:proves subagent-lifecycle-hooks/codex
func TestSubagentHookMatcherAndContinueFalse(t *testing.T) {
	group := func(matcher string) string {
		return `[{"matcher":"` + matcher + `","hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh"}]}]`
	}
	got := execMock(t, scenario{
		HooksJSON: `{"hooks":{"SubagentStart":` + group("default") + `,"SubagentStop":` + group("nomatch") + `}}`,
		Files: map[string]string{"sub.sh": subScript,
			"hook.sh": "#!/bin/sh\ncat >>\"$HOOK_LOG\"\necho >>\"$HOOK_LOG\"\necho '{\"continue\":false}'\n"},
		Script: spawnThenResult, Prompt: "go",
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	have := byEvent(got.hookLog())
	assert.Len(t, have["SubagentStart"], 1, "its matcher matches the sub-agent's type")
	assert.Empty(t, have["SubagentStop"], "its matcher does not")
	assert.Contains(t, got.Stdout, `"message":"SUB-DONE"`, "the sub-agent ran to its end whatever the start hook said")
}

// What a start hook prints, as plain text or as additionalContext, is developer
// context in the sub-agent's own rollout, not the session's; and SessionEnd,
// which does not run for sub-agents, fires once (hooks#subagentstart, hooks#sessionend).
// sr:proves subagent-lifecycle-hooks/codex
// sr:proves session-end-hook/codex
func TestSubagentStartContextAndNoSessionEndForIt(t *testing.T) {
	cmd := `[{"hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh"}]}]`
	hook := "#!/bin/sh\ncat >>\"$HOOK_LOG\"\necho >>\"$HOOK_LOG\"\n" +
		`if [ "$1" = json ]; then echo '{"hookSpecificOutput":{"hookEventName":"SubagentStart","additionalContext":"CTX-JSON"}}'; else echo CTX-PLAIN; fi` + "\n"
	for form, want := range map[string]string{"plain": "CTX-PLAIN", "json": "CTX-JSON"} {
		t.Run(form, func(t *testing.T) {
			got := execMock(t, scenario{
				HooksJSON: strings.ReplaceAll(`{"hooks":{"SubagentStart":C,"SessionEnd":C}}`, "C", strings.ReplaceAll(cmd, "hook.sh", "hook.sh "+form)),
				Files:     map[string]string{"sub.sh": subScript, "hook.sh": hook},
				Script:    spawnThenResult, Prompt: "go",
			})
			require.Equal(t, 0, got.Code, got.Stderr)
			var sub, session string
			require.NoError(t, filepath.Walk(filepath.Join(got.Home, "sessions"), func(p string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					b, _ := os.ReadFile(p)
					if strings.Contains(string(b), "spawn_agent") {
						session = string(b)
					} else {
						sub = string(b)
					}
				}
				return nil
			}))
			assert.Contains(t, sub, want, "the sub-agent's rollout")
			assert.NotContains(t, session, want, "not the session's")
			assert.Len(t, byEvent(got.hookLog())["SessionEnd"], 1, "SessionEnd fires for the session only")
		})
	}
}
