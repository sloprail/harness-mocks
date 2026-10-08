package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// lastSaid is the agent's last assistant message in a recorded stream.
func lastSaid(t *testing.T, sample string) string {
	t.Helper()
	var said string
	for _, f := range readJSONL(t, filepath.Join(sample, "stream.jsonl")) {
		if f["type"] != "assistant" {
			continue
		}
		msg, _ := f["message"].(map[string]any)
		content, _ := msg["content"].([]any)
		for _, c := range content {
			if b, _ := c.(map[string]any); b["type"] == "text" {
				said, _ = b["text"].(string)
			}
		}
	}
	return said
}

// TestAHooksAdditionalContextReachesTheAgentFromTheStartAndAfterEveryToolCall:
// recorded (runs/additional-context: hooks that answer with an
// additional_context naming themselves, asked to be repeated back), the agent
// has the context of the sessionStart hook, of both postToolUse hooks after a
// call that succeeded, and of the postToolUseFailure hook after one that
// failed: all of it, the hooks' in the order they are configured. Cursor does
// not write the context into the transcript.
// sr:proves hook-additional-context/cursor
func TestAHooksAdditionalContextReachesTheAgentFromTheStartAndAfterEveryToolCall(t *testing.T) {
	// what the mock shows of the recorded run is compared by the generated replay
	// (e2e/003_replay): the hand-written comparison of the hook scripts' own logs
	// lost lines when hooks ran side by side under load
	sample := newestSample(t, "additional-context")
	require.Equal(t, "CTX-sessionStart-S1 CTX-postToolUse-P1 CTX-postToolUse-P2 CTX-postToolUseFailure-F1", lastSaid(t, sample),
		"the recorded agent had all of the context, in the order it was added")
	transcripts, err := filepath.Glob(filepath.Join(sample, "transcript", "*", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, transcripts, 1)
	b, err := os.ReadFile(transcripts[0])
	require.NoError(t, err)
	for _, given := range strings.Fields(lastSaid(t, sample)) {
		require.Equal(t, 1, strings.Count(string(b), given), given+": only the agent's own words repeat it, the transcript holds no hook context of its own")
	}

	// the mock hands the scripted agent the same, as it goes: what it has been
	// given is in A10N_MOCK_ADDITIONAL_CONTEXT on each of its runs
	ctx := func(arg string) string {
		return `{"command":".cursor/hooks/ctx.sh ` + arg + `"}`
	}
	c := runCustom(t, `{"version":1,"hooks":{"sessionStart":[`+ctx("S1")+`],"postToolUse":[`+ctx("P1")+`,`+ctx("P2")+`],"postToolUseFailure":[`+ctx("F1")+`]}}`,
		map[string]string{"ctx.sh": "#!/bin/sh\ncat >/dev/null\nprintf '{\"additional_context\":\"CTX-%s\"}\\n' \"$1\"\n"},
		"echo FIRST", "grep zzz /dev/null")
	seen, err := os.ReadFile(c.log + ".ctx")
	require.NoError(t, err)
	var runs []string
	for _, l := range strings.Split(strings.TrimSpace(string(seen)), "\n") {
		runs = append(runs, strings.Join(strings.Fields(l), " "))
	}
	require.Equal(t, []string{
		"CTX-S1",
		"CTX-S1 CTX-P1 CTX-P2",
		"CTX-S1 CTX-P1 CTX-P2 CTX-F1",
	}, runs, "what the agent had before each of its steps")
}

// TestAPostToolUseFailureHooksContextAlsoReachesTheAgentAfterARefusedCall: docs
// (#posttoolusefailure, failure_type permission_denied), a call a preToolUse
// hook refused counts as a failed tool, and the additional_context of the
// postToolUseFailure hook is handed to the agent as after any other failure.
// Not recorded: the mock's reading of the docs. A tool that timed out is not
// modeled (a failure is only an error or a refusal).
// sr:proves hook-additional-context/cursor
func TestAPostToolUseFailureHooksContextAlsoReachesTheAgentAfterARefusedCall(t *testing.T) {
	c := runCustom(t, `{"version":1,"hooks":{"preToolUse":[{"command":".cursor/hooks/deny.sh"}],"postToolUseFailure":[{"command":".cursor/hooks/ctx.sh"}]}}`,
		map[string]string{
			"deny.sh": "#!/bin/sh\ncat >/dev/null\necho '{\"permission\":\"deny\",\"user_message\":\"NO\"}'\n",
			"ctx.sh":  "#!/bin/sh\ncat >/dev/null\necho '{\"additional_context\":\"CTX-DENIED\"}'\n",
		}, "echo REFUSED")
	seen, err := os.ReadFile(c.log + ".ctx")
	require.NoError(t, err)
	var runs []string
	for _, l := range strings.Split(strings.TrimSpace(string(seen)), "\n") {
		runs = append(runs, strings.Join(strings.Fields(l), " "))
	}
	// the agent's first step had nothing (a blank line, trimmed above); its next had the context
	require.Equal(t, []string{"CTX-DENIED"}, runs, "what the agent had at its step after the refusal")
}
