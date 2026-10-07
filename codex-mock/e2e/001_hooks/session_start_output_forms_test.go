package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What a SessionStart hook prints becomes one developer message, by the form of its
// output: plain text is the message as printed; a JSON object with
// hookSpecificOutput.additionalContext is the message of just that string; output
// that starts with plain text and then holds the JSON is not parsed, and the whole
// stdout (the plain line and the raw JSON text) is the message (recorded:
// runs/session-start-plain-context, runs/session-start-json-context,
// runs/session-start-both-context, each a project-layer hook in a trusted project).
// sr:proves hook-additional-context/codex
// sr:proves session-start-hook/codex
func TestSessionStartOutputFormsBecomeOneDeveloperMessage(t *testing.T) {
	for run, want := range map[string]string{
		"session-start-plain-context": "CTX-PLAIN-1",
		"session-start-json-context":  "CTX-JSON-2",
		"session-start-both-context":  `CTX-PLAIN-3` + "\n" + `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"CTX-JSON-4"}}`,
	} {
		t.Run(run, func(t *testing.T) {
			rec := loadRecording(t, run)
			got := execMock(t, scenario{
				ProjectHooksJSON: readFile(t, filepath.Join(rec.setup, "project-hooks.json")),
				Files:            map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
				Script:           callThenResult, Prompt: strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
				Env: withCalls(t),
			})
			require.Equal(t, 0, got.Code, got.Stderr)
			assert.Equal(t, []string{want}, addedContext(t, recordedRollout(t, rec)), "recorded")
			assert.Equal(t, []string{want}, addedContext(t, got.rollout(t)), "the mock's")
		})
	}
}
