package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/subagent-stop-block-loop: one sub-agent whose
// SubagentStop hook blocks by a JSON decision, then by exit 2, then lets it
// stop; the hook is also given SubagentStart, Stop and SessionStart.
const subagentRun = "subagent-stop-block-loop"

// rollouts are the session files a run left, the sub-agent's (the one that
// holds a stop hook's feedback, or just the second) and the session's.
func rollouts(t *testing.T, home string) (all []string) {
	t.Helper()
	require.NoError(t, filepath.Walk(filepath.Join(home, "sessions"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			all = append(all, string(b))
		}
		return nil
	}))
	return all
}

// feedbackOf is the stop hook feedback a rollout records as user messages, in
// order, each with the number of assistant messages recorded before it.
func feedbackOf(rollout string) (reasons []string, afterN []int) {
	n := 0
	for _, l := range jsonLines(rollout) {
		p, _ := l["payload"].(map[string]any)
		content, _ := p["content"].([]any)
		if p["type"] != "message" || len(content) == 0 {
			continue
		}
		text, _ := content[0].(map[string]any)["text"].(string)
		switch {
		case p["role"] == "assistant":
			n++
		case p["role"] == "user" && strings.HasPrefix(text, "<hook_prompt"):
			reasons = append(reasons, strings.TrimSuffix(text[strings.LastIndex(text, `">`)+2:], "</hook_prompt>"))
			afterN = append(afterN, n)
		}
	}
	return
}

// A sub-agent's stop hook that blocks (by a block decision or exit 2) runs the
// sub-agent again with the hook's reason as feedback, until the hook lets it
// stop; the feedback is recorded in the sub-agent's own rollout, never the
// session's, and the sub-agent's stop is fired again each time with
// stop_hook_active (runs/subagent-stop-block-loop).
// sr:proves subagent-stop-block-loop/codex
func TestSubagentStopBlockRunsTheSubagentAgain(t *testing.T) {
	samples, err := filepath.Glob(filepath.Join(runsDir, subagentRun, "samples", "*"))
	require.NoError(t, err)
	require.NotEmpty(t, samples)
	sample := samples[len(samples)-1]
	setup := filepath.Join(runsDir, subagentRun, "setup")

	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(setup, "hook.sh")), "sub.sh": subScript},
		Script:    spawnThenResult, Prompt: strings.TrimSpace(readFile(t, filepath.Join(setup, "prompt.txt"))),
	})
	require.Equal(t, 0, got.Code, got.Stderr)

	// the hooks fired in the recorded order, the stop's active flag as recorded
	shape := func(lines []map[string]any) (out []any) {
		for _, l := range lines {
			out = append(out, []any{l["hook_event_name"], l["stop_hook_active"]})
		}
		return
	}
	want := shape(jsonLines(readFile(t, filepath.Join(sample, "payloads.jsonl"))))
	require.Len(t, want, 6)
	assert.Equal(t, want, shape(got.hookLog()))

	// the feedback is the hook's reasons, in order, in the sub-agent's own rollout
	var wantRolls []string
	files, _ := filepath.Glob(filepath.Join(sample, "transcript", "*"))
	for _, f := range files {
		wantRolls = append(wantRolls, readFile(t, f))
	}
	var wantReasons []string
	var wantAfter []int
	for _, r := range wantRolls {
		if reasons, after := feedbackOf(r); len(reasons) > 0 {
			wantReasons, wantAfter = reasons, after
		}
	}
	require.Equal(t, []string{"SUBSTOP-REASON-1", "SUBSTOP-REASON-2"}, wantReasons)
	var gotReasons []string
	var gotAfter []int
	for _, r := range rollouts(t, got.Home) {
		reasons, after := feedbackOf(r)
		if len(reasons) > 0 {
			require.Empty(t, gotReasons, "feedback in two rollouts")
			gotReasons, gotAfter = reasons, after
		}
	}
	assert.Equal(t, wantReasons, gotReasons, "the feedback in the sub-agent's own rollout")
	assert.Equal(t, wantAfter, gotAfter, "the sub-agent ran again after each block")

	// the session's own rollout holds none of it, and the caller sees one end
	for _, r := range rollouts(t, got.Home) {
		if strings.Contains(r, "spawn_agent") {
			assert.NotContains(t, r, "SUBSTOP-REASON")
		}
	}
	assert.Equal(t, 1, strings.Count(got.Stdout, `"type":"turn.completed"`))
}

// The sub-agent's stop is told its own rollout (agent_transcript_path), the
// one that holds the feedback, beside the session's transcript_path; and
// SessionEnd, which the doc says does not run for sub-agents, fires once, for
// the session, after everything else (runs/subagent-stop-block-loop).
// sr:proves subagent-stop-block-loop/codex
func TestSubagentStopNamesTheSubagentsOwnRollout(t *testing.T) {
	setup := filepath.Join(runsDir, subagentRun, "setup")
	got := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "SubagentStop", "SessionEnd"),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(setup, "hook.sh")), "sub.sh": subScript},
		Script:    spawnThenResult, Prompt: "go",
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	var events []any
	for _, l := range got.hookLog() {
		events = append(events, l["hook_event_name"])
		if l["hook_event_name"] != "SubagentStop" {
			continue
		}
		own, _ := l["agent_transcript_path"].(string)
		assert.NotEqual(t, l["transcript_path"], own, "the sub-agent's rollout is not the session's")
		assert.Contains(t, readFile(t, own), "SUBSTOP-REASON-1", "the path names the rollout holding the feedback")
	}
	assert.Equal(t, []any{"SubagentStop", "SubagentStop", "SubagentStop", "SessionEnd"}, events)
}

// The payloads of the sub-agent's hooks name it (agent_id, agent_type), the
// session and the turn, as the recording's do, and SubagentStop carries the
// sub-agent's last message; the feedback is wrapped as a hook_prompt user
// message in the recording and the mock alike. A matcher that does not select
// the agent_type keeps the hook from running, so the sub-agent is not re-run
// (runs/subagent-stop-block-loop).
// sr:proves subagent-stop-block-loop/codex
func TestSubagentHookPayloadsAndMatcher(t *testing.T) {
	setup := filepath.Join(runsDir, subagentRun, "setup")
	samples, _ := filepath.Glob(filepath.Join(runsDir, subagentRun, "samples", "*"))
	require.NotEmpty(t, samples)
	run := func(hooks string) result {
		return execMock(t, scenario{HooksJSON: hooks,
			Files:  map[string]string{"hook.sh": readFile(t, filepath.Join(setup, "hook.sh")), "sub.sh": subScript},
			Script: spawnThenResult, Prompt: "go"})
	}
	got := run(readFile(t, filepath.Join(setup, "hooks.json")))
	require.Equal(t, 0, got.Code, got.Stderr)

	pick := func(lines []map[string]any, ev string) (out []map[string]any) {
		for _, l := range lines {
			if l["hook_event_name"] == ev {
				out = append(out, l)
			}
		}
		return
	}
	want := jsonLines(readFile(t, filepath.Join(samples[len(samples)-1], "payloads.jsonl")))
	for _, ev := range []string{"SubagentStart", "SubagentStop"} {
		w, g := pick(want, ev), pick(got.hookLog(), ev)
		require.Len(t, g, len(w), ev)
		for i := range g {
			for _, k := range []string{"agent_type", "permission_mode"} {
				assert.Equal(t, w[i][k], g[i][k], ev+" "+k)
			}
			assert.NotEmpty(t, g[i]["agent_id"])
			assert.Equal(t, g[0]["agent_id"], g[i]["agent_id"], "one sub-agent")
			assert.NotEmpty(t, g[i]["session_id"])
			assert.NotEmpty(t, g[i]["turn_id"])
			if ev == "SubagentStop" {
				assert.Equal(t, "SUB-DONE", g[i]["last_assistant_message"])
			}
		}
	}
	for _, r := range rollouts(t, got.Home) {
		if reasons, _ := feedbackOf(r); len(reasons) > 0 {
			assert.Contains(t, r, `<hook_prompt hook_run_id=\"subagent-stop`)
		}
	}

	skipped := run(`{"hooks":{"SubagentStop":[{"matcher":"explorer","hooks":[{"type":"command","command":"sh hook.sh"}]}]}}`)
	require.Equal(t, 0, skipped.Code, skipped.Stderr)
	assert.Empty(t, skipped.hookLog(), "a matcher that does not select the agent type ran the hook")
	for _, r := range rollouts(t, skipped.Home) {
		assert.NotContains(t, r, "hook_prompt")
	}
}
