package e2e

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookLabels are what a hook log shows of a run, one label per line: the event of a
// payload, or the hook that logged its own result; sorted, since hooks of one event
// run at the same time.
func hookLabels(lines []map[string]any) (out []string) {
	for _, l := range lines {
		switch {
		case l["hook_event_name"] != nil:
			label := fmt.Sprint(l["hook_event_name"])
			if a, ok := l["stop_hook_active"]; ok {
				label += fmt.Sprintf(" active=%v", a)
			}
			out = append(out, label)
		case l["hook_result"] != nil:
			r := l["hook_result"].(map[string]any)
			out = append(out, fmt.Sprintf("result %v", r["hook"]))
		}
	}
	sort.Strings(out)
	return
}

// runSubagentStop replays a recorded run's hooks on a spawn, a wait and an answer,
// the sub-agent answering PONG.
func runSubagentStop(t *testing.T, name string) (recorded, got []map[string]any) {
	t.Helper()
	rec := loadRecording(t, name)
	recorded = jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))
	r := execMock(t, scenario{Script: pingSpawn,
		Files:     map[string]string{"pong.sh": pongScript, "hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")), Prompt: "go"})
	require.Equal(t, 0, r.Code, r.Stderr)
	return recorded, r.hookLog()
}

// continue:false of one SubagentStop hook takes precedence over another's block: the
// sub-agent is not continued, and both hooks ran (recorded: runs/subagent-stop-continue-false).
// sr:proves hook-exit-code-semantics/codex
// sr:proves subagent-lifecycle-hooks/codex
func TestASubagentStopContinueFalseBeatsAnotherHooksBlock(t *testing.T) {
	recorded, got := runSubagentStop(t, "subagent-stop-continue-false")
	assert.Equal(t, hookLabels(recorded), hookLabels(got))
	assert.Zero(t, countLabel(hookLabels(got), "SubagentStop active=true"), "the sub-agent was stopped, not run again")
}

// Plain text on stdout with exit 0 is invalid for SubagentStop: it is ignored and the
// sub-agent stops (recorded: runs/subagent-stop-plain-text).
// sr:proves hook-exit-code-semantics/codex
// sr:proves subagent-lifecycle-hooks/codex
func TestASubagentStopPlainTextIsIgnored(t *testing.T) {
	recorded, got := runSubagentStop(t, "subagent-stop-plain-text")
	assert.Equal(t, hookLabels(recorded), hookLabels(got))
	assert.Zero(t, countLabel(hookLabels(got), "SubagentStop active=true"), "the sub-agent was stopped, not continued")
}

func countLabel(labels []string, want string) (n int) {
	for _, l := range labels {
		if l == want {
			n++
		}
	}
	return
}

// What a SubagentStart hook prints as JSON, a systemMessage included, steers and shows nothing: the
// sub-agent runs and stops, and the message is in neither the stream nor the transcript (recorded:
// runs/subagent-start-systemmessage).
// sr:proves subagent-lifecycle-hooks/codex
func TestASubagentStartSystemMessageIsNotShown(t *testing.T) {
	rec, got := replaySubagent(t, "subagent-start-systemmessage")
	require.Equal(t, 0, got.Code, got.Stderr)
	recordedStream := readFile(t, filepath.Join(rec.sample, "stream.jsonl"))
	assert.NotContains(t, recordedStream, "SA-SYSMSG")
	files, _ := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	require.Len(t, files, 2, "the session's and the sub-agent's")
	for _, f := range files {
		assert.NotContains(t, readFile(t, f), "SA-SYSMSG")
	}
	assert.Equal(t, []string{"SubagentStart", "SubagentStop"}, eventNames(got.hookLog()), "the sub-agent ran and stopped")
	assert.NotContains(t, got.Stdout, "SA-SYSMSG")
	assert.NotContains(t, got.Stderr, "SA-SYSMSG")
	assert.NotContains(t, got.rollout(t), "SA-SYSMSG")
}

// A sub-agent that ends with no message stops with last_assistant_message null, and the wait
// reports it completed with null, though it has a transcript of its own (recorded:
// runs/subagent-stop-no-message).
// sr:proves subagent-lifecycle-hooks/codex
func TestASubagentThatEndsWithNoMessageStopsWithANullMessage(t *testing.T) {
	rec := loadRecording(t, "subagent-stop-no-message")
	stop := func(log []map[string]any) map[string]any {
		var out map[string]any
		for _, l := range log {
			if l["hook_event_name"] == "SubagentStop" {
				out = l
			}
		}
		require.NotNil(t, out)
		return out
	}
	recorded := stop(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	assert.Contains(t, recorded, "last_assistant_message", "recorded: the key is there")
	assert.Nil(t, recorded["last_assistant_message"], "recorded: and explicitly null")
	assert.NotNil(t, recorded["agent_transcript_path"])

	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files: map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh")),
			"sub.sh": "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"\"}]}}' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"\"}'\n"},
		Script: spawnThenResult, Prompt: "go",
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	mock := stop(got.hookLog())
	assert.Contains(t, mock, "last_assistant_message", "the mock's: the key is there")
	assert.Nil(t, mock["last_assistant_message"], "the mock's: and explicitly null")
	assert.NotNil(t, mock["agent_transcript_path"])
	assert.Contains(t, got.rollout(t), `\"completed\":null`)
}

// A PreToolUse or PostToolUse matcher on one of the sub-agent tools matches that tool's own name
// (hooks#tool-coverage): a matcher of spawn_agent runs for the spawn and not for the sub-agent's own
// shell command or the wait, and one of wait_agent for the wait only, whose name in the payload is
// the one the harness gives it (recorded: runs/foreground-subagent-result, tool_name
// "multi_agent_v1wait_agent").
// sr:proves hook-matcher-filter/codex
func TestAToolMatcherOnASubagentToolMatchesThatToolsName(t *testing.T) {
	rec := loadRecording(t, "foreground-subagent-result")
	var names []string
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if n, ok := l["tool_name"].(string); ok {
			names = append(names, n)
		}
	}
	assert.Contains(t, names, "spawn_agent")
	assert.Contains(t, names, "multi_agent_v1wait_agent")

	for matcher, want := range map[string]string{"spawn_agent": "spawn_agent", "wait_agent": "multi_agent_v1wait_agent"} {
		t.Run(matcher, func(t *testing.T) {
			group := func(ev string) string {
				return `"` + ev + `":[{"matcher":"` + matcher + `","hooks":[{"type":"command","command":"cat >>\"$HOOK_LOG\"; echo >>\"$HOOK_LOG\""}]}]`
			}
			got := execMock(t, scenario{
				HooksJSON: `{"hooks":{` + group("PreToolUse") + `,` + group("PostToolUse") + `}}`,
				Files:     map[string]string{"sub.sh": subScript},
				Script:    spawnThenResult, Prompt: "go",
			})
			require.Equal(t, 0, got.Code, got.Stderr)
			var seen []string
			for _, l := range got.hookLog() {
				seen = append(seen, fmt.Sprint(l["hook_event_name"], ":", l["tool_name"]))
			}
			sort.Strings(seen)
			wantEvents := []string{"PostToolUse:" + want, "PreToolUse:" + want}
			assert.Equal(t, wantEvents, seen, "the hooks ran for that tool's calls only: not for the sub-agent's Bash")
		})
	}
}

// A SubagentStop hook with a matcher that matches the sub-agent's type runs (the type is "default"),
// by the name and by a regex, and one that does not match does not (hooks#subagentstop).
// sr:proves hook-matcher-filter/codex
func TestASubagentStopMatcherThatMatchesTheTypeRuns(t *testing.T) {
	for matcher, runs := range map[string]bool{"default": true, "^def.*": true, "def|other": true, "explorer": false} {
		t.Run(matcher, func(t *testing.T) {
			got := execMock(t, scenario{
				HooksJSON: `{"hooks":{"SubagentStop":[{"matcher":"` + matcher + `","hooks":[{"type":"command","command":"cat >>\"$HOOK_LOG\"; echo >>\"$HOOK_LOG\""}]}]}}`,
				Files:     map[string]string{"sub.sh": subScript},
				Script:    spawnThenResult, Prompt: "go",
			})
			require.Equal(t, 0, got.Code, got.Stderr)
			stops := byEvent(got.hookLog())["SubagentStop"]
			assert.Equal(t, runs, len(stops) == 1, "matcher %q", matcher)
			if runs {
				assert.Equal(t, "default", stops[0]["agent_type"])
			}
		})
	}
}

// A SubagentStop hook that exits 2 and also prints continue:false blocks: the exit status wins over
// the JSON, so the sub-agent is run again with the stderr reason, as it is for a block decision
// (recorded: runs/subagent-stop-exit2-continue-false: three stops, the first two blocked).
// sr:proves hook-exit-code-semantics/codex
// sr:proves subagent-lifecycle-hooks/codex
func TestASubagentStopExit2BeatsContinueFalse(t *testing.T) {
	recorded, got := runSubagentStop(t, "subagent-stop-exit2-continue-false")
	assert.Equal(t, hookLabels(recorded), hookLabels(got))
	assert.Equal(t, 2, countLabel(hookLabels(got), "SubagentStop active=true"), "the sub-agent was run again twice")
	assert.Equal(t, 1, countLabel(hookLabels(got), "SubagentStop active=false"))
}
