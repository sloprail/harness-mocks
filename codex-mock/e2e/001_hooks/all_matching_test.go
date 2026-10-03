package e2e

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ran is the handlers the run's hooks logged as having run, sorted.
func ran(r result) []string { return names(r.hookLog()) }

// recorded is the same, from a recorded run's payloads.jsonl.
func recorded(t *testing.T, rec recording) []string {
	return names(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
}

func names(lines []map[string]any) (out []string) {
	for _, l := range lines {
		out = append(out, l["ran"].(string))
	}
	sort.Strings(out)
	return
}

// Two before-tool hooks both refuse one call, one slow and one fast, in either
// order of configuration: both run to completion, and the reason the agent is
// told is that of the one configured first, not of the one that finished last
// (recorded in runs/hooks-all-matching-run-blockers-slow-first and -fast-first).
// sr:proves hooks-all-matching-run/codex
func TestAllBlockersRunAndTheFirstConfiguredIsActedOn(t *testing.T) {
	for name, first := range map[string]string{
		"hooks-all-matching-run-blockers-slow-first": "slow",
		"hooks-all-matching-run-blockers-fast-first": "fast",
	} {
		t.Run(name, func(t *testing.T) {
			rec := loadRecording(t, name)
			rec.calls = []string{"echo BLOCKME"} // the command the prompt names; the hooks log no payload
			// what the real Codex logged and told
			assert.Equal(t, []string{"fast", "slow"}, recorded(t, rec))
			assert.Contains(t, readFile(t, filepath.Join(rec.sample, "stderr.txt")), "denied by "+first+". Command: echo BLOCKME")

			got := replay(t, rec)
			require.Equal(t, 0, got.Code, got.Stderr)
			assert.Equal(t, []string{"fast", "slow"}, ran(got), "a hook did not run")
			assert.Contains(t, got.Stderr, "Command blocked by PreToolUse hook: denied by "+first+". Command: echo BLOCKME")
			cmds, _ := got.commands()
			assert.Empty(t, cmds, "the refused command ran")
		})
	}
}

// The same hook configured in the user layer and in the project layer runs
// twice, once per file: Codex does not fold a hook listed in two files into
// one, and the project's other hook runs too (runs/hooks-all-matching-run-same-hook-two-files).
// sr:proves hooks-all-matching-run/codex
func TestSameHookInTwoFilesRunsOncePerFile(t *testing.T) {
	rec := loadRecording(t, "hooks-all-matching-run-same-hook-two-files")
	want := []string{"project-only", "same", "same"}
	assert.Equal(t, want, recorded(t, rec))

	got := execMock(t, scenario{
		HooksJSON:        readFile(t, filepath.Join(rec.setup, "hooks.json")),
		ProjectHooksJSON: readFile(t, filepath.Join(rec.setup, "project-hooks.json")),
		Files:            map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:           callThenResult,
		Prompt:           strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:              withCalls(t, rec.calls...),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, want, ran(got))
}

// Every hook of the event runs for every call, whatever the others decide:
// three hooks (allow, deny, ask) on each of three commands are nine runs, three
// per command, the denied command's included (runs/pretool-decisions).
// sr:proves hooks-all-matching-run/codex
func TestEveryMatchingHookRunsWhateverTheOthersDecide(t *testing.T) {
	rec := loadRecording(t, "pretool-decisions")
	perCommand := func(lines []map[string]any) map[string]int {
		n := map[string]int{}
		for _, l := range lines {
			in, _ := l["tool_input"].(map[string]any)
			if l["hook_event_name"] == "PreToolUse" {
				n[in["command"].(string)]++
			}
		}
		return n
	}
	want := map[string]int{"echo ONLYALLOW": 3, "echo ALLOWDENY": 3, "echo ASKME": 3}
	assert.Equal(t, want, perCommand(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl")))))

	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, want, perCommand(got.hookLog()))
	assert.Contains(t, got.Stderr, "Command blocked by PreToolUse hook: deny by hook deny. Command: echo ALLOWDENY")
}
