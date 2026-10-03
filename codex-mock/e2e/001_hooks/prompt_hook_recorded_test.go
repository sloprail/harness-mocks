package e2e

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func payloadKeys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// recordedHooks are a recorded sample's hook payloads, by event name.
func recordedHooks(t *testing.T, rec recording, event string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))) {
		if l["hook_event_name"] == event {
			out = append(out, l)
		}
	}
	return out
}

// The recorded run runs/user-prompt-submit-hook-rewrite: a UserPromptSubmit
// hook asks 'Reply only with the word ORIGINAL.' and prints JSON naming a
// replacement prompt in every shape a rewrite could take (prompt,
// updatedPrompt, updatedInput, in the top level and in hookSpecificOutput).

// TestAUserPromptSubmitHookCannotRewriteThePrompt: recorded, the agent
// answered the original prompt and the rollout holds only it, the hook's
// replacement is nowhere. The mock does the same.
// sr:proves user-prompt-submit-hook/codex
func TestAUserPromptSubmitHookCannotRewriteThePrompt(t *testing.T) {
	rec := loadRecording(t, "user-prompt-submit-hook-rewrite")
	sample := readFile(t, filepath.Join(rec.sample, "stream.jsonl"))
	assert.Contains(t, sample, `"text":"ORIGINAL"`, "the agent answered the original prompt")
	assert.NotContains(t, sample, "REWRITTEN")
	require.Len(t, recordedHooks(t, rec, "UserPromptSubmit"), 1)
	assert.Equal(t, "Reply only with the word ORIGINAL.", recordedHooks(t, rec, "UserPromptSubmit")[0]["prompt"])

	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Contains(t, got.rollout(t), "Reply only with the word ORIGINAL.")
	assert.NotContains(t, got.rollout(t), "REWRITTEN")
	assert.Equal(t, "Reply only with the word ORIGINAL.", eventsOf(got, "UserPromptSubmit")[0]["prompt"])
}

// The recorded run runs/user-prompt-submit-hook-bg: the agent starts a
// background command and ends its turn; a UserPromptSubmit and a Stop hook
// log their payloads.

// TestUserPromptSubmitFiresOnlyForTheSubmittedPromptNotForABackgroundCommand:
// recorded, the hook fired once, for the submitted prompt: nothing about the
// background command ending was handed to the agent as a prompt, so the hook
// had none to see. The mock fires it once for the one prompt too.
// sr:proves user-prompt-submit-hook/codex
func TestUserPromptSubmitFiresOnlyForTheSubmittedPromptNotForABackgroundCommand(t *testing.T) {
	rec := loadRecording(t, "user-prompt-submit-hook-bg")
	require.Len(t, recordedHooks(t, rec, "UserPromptSubmit"), 1)
	require.Len(t, recordedHooks(t, rec, "Stop"), 1)

	rec.calls = []string{"sleep 1 &"} // the recorded command, started in the background and not waited on
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Len(t, eventsOf(got, "UserPromptSubmit"), 1)
}

// TestTheStopPayloadOfARunWithABackgroundCommandCarriesNoBackgroundTasks:
// recorded, the Stop hook of a turn that started a background command gets the
// same fields as any other: the last message and whether the turn is
// continuing, and no list of background tasks. The mock's Stop payload has the
// same fields.
// sr:proves stop-hook-payload/codex
func TestTheStopPayloadOfARunWithABackgroundCommandCarriesNoBackgroundTasks(t *testing.T) {
	rec := loadRecording(t, "user-prompt-submit-hook-bg")
	stops := recordedHooks(t, rec, "Stop")
	require.Len(t, stops, 1)
	assert.Equal(t, "LAUNCHED", stops[0]["last_assistant_message"])
	assert.Equal(t, false, stops[0]["stop_hook_active"])
	want := payloadKeys(stops[0])
	for _, k := range want {
		assert.NotContains(t, k, "task", "no field of a recorded Stop payload names background tasks")
	}

	rec.calls = []string{"sleep 1 &"}
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)
	mock := eventsOf(got, "Stop")
	require.Len(t, mock, 1)
	assert.Equal(t, want, payloadKeys(mock[0]), "the mock's Stop payload has the recorded fields")
}
