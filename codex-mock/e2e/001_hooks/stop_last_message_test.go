package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAStopAfterABlockCarriesTheMessageTheAgentSaidLast: recorded in
// runs/stops, the first Stop carries DONE and the two Stops that follow a
// block carry DONE2, what the agent said after being told to continue, not
// the first message again. The mock, whose agent says DONE and then, once told
// to continue, DONE2, carries the same sequence.
// sr:proves stop-hook-payload/codex
func TestAStopAfterABlockCarriesTheMessageTheAgentSaidLast(t *testing.T) {
	var want []any
	for _, s := range recordedHooks(t, loadRecording(t, "stops"), "Stop") {
		want = append(want, s["last_assistant_message"])
	}
	require.Equal(t, []any{"DONE", "DONE2", "DONE2"}, want)

	script := `#!/bin/sh
if grep -q hook_prompt "$A10N_MOCK_SESSION_FILE"; then m=DONE2; else m=DONE; fi
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"'$m'"}]}}' '{"type":"result","subtype":"success","result":"'$m'"}'
`
	r := execMock(t, scenario{
		HooksJSON: hooksJSON("sh hook.sh", "Stop"),
		Files: map[string]string{"hook.sh": `cat >>"$HOOK_LOG"; echo >>"$HOOK_LOG"
n=$(cat "$TMPDIR/n" 2>/dev/null || echo 0); n=$((n+1)); echo $n >"$TMPDIR/n"
if [ $n -le 2 ]; then echo again >&2; exit 2; fi`},
		Script: script, Prompt: "go", Env: withCalls(t),
	})
	require.Equal(t, 0, r.Code, r.Stderr)
	var got []any
	for _, s := range eventsOf(r, "Stop") {
		got = append(got, s["last_assistant_message"])
	}
	assert.Equal(t, want, got)
}
