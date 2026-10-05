package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Several hooks on one event each add their text, and all of it reaches the agent as
// developer messages in the order the hooks are configured, whatever the event
// (SessionStart, UserPromptSubmit, or PostToolUse by JSON additionalContext).
// sr:proves hook-additional-context/codex
func TestSeveralHooksOnOneEventEachAddTheirText(t *testing.T) {
	two := func(event, a, b string) string {
		j := func(text string) string {
			return `{"type":"command","command":"echo '{\"hookSpecificOutput\":{\"hookEventName\":\"` + event + `\",\"additionalContext\":\"` + text + `\"}}'"}`
		}
		return `"` + event + `":[{"hooks":[` + j(a) + `,` + j(b) + `]}]`
	}
	got := execMock(t, scenario{
		HooksJSON: `{"hooks":{` + two("SessionStart", "START-1", "START-2") + `,` + two("UserPromptSubmit", "PROMPT-1", "PROMPT-2") + `,` + two("PostToolUse", "POST-1", "POST-2") + `}}`,
		Script:    callThenResult, Prompt: "go", Env: withCalls(t, "true"),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, []string{"START-1", "START-2", "PROMPT-1", "PROMPT-2", "POST-1", "POST-2"}, developerTexts(t, got.rollout(t)))
}

// What a SessionStart hook adds when the session starts again after a compaction
// (source "compact") is kept too, after the compaction's record, before the
// agent goes on (hooks#sessionstart).
// sr:proves hook-additional-context/codex
func TestSessionStartContextAfterACompactionIsKept(t *testing.T) {
	got := execMock(t, scenario{
		HooksJSON: `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"sh hook.sh"}]}]}}`,
		Files: map[string]string{"hook.sh": `in=$(cat); case "$in" in
  *'"source":"compact"'*) echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"CTX-COMPACT"}}' ;;
esac`},
		Script: `#!/bin/sh
c=$(grep -c '"type":"compacted"' "$A10N_MOCK_SESSION_FILE")
case "$c" in
0) printf '%s\n' '{"type":"compact","trigger":"auto"}' ;;
*) printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}' ;;
esac
`,
		Prompt: "go",
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, []string{"CTX-COMPACT"}, developerTexts(t, got.rollout(t)), "only the compact start added context")
	rollout := got.rollout(t)
	assert.Less(t, strings.Index(rollout, `"type":"compacted"`), strings.Index(rollout, "CTX-COMPACT"), "after the compaction's record")
}

// A PostToolUse hook that blocks by JSON and adds context in the same answer has both
// kept: the reason replaces the call's output, and the context is a developer message
// (recorded: runs/posttool-block-context, the doc's hooks#posttooluse example).
// sr:proves hook-additional-context/codex
func TestPostToolUseBlockAndAddedContextAreBothKept(t *testing.T) {
	rec := loadRecording(t, "posttool-block-context")
	for name, rollout := range map[string]string{"recorded": recordedRollout(t, rec), "mock": replay(t, rec).rollout(t)} {
		assert.True(t, resultTold(t, rollout, "POST-REASON-MSG", "POSTBLOCK"), name+": the reason replaces the output")
		assert.Equal(t, []string{"POST-CONTEXT-MSG"}, addedContext(t, rollout), name+": the context is kept")
	}
}

// What a SessionStart hook adds when the session is resumed (source "resume") is kept as
// developer context in the session's record after the startup's, in the same file; the
// hook matching only startup does not run again (recorded: runs/session-resume-context).
// sr:proves hook-additional-context/codex
func TestSessionStartContextOnAResumedSessionIsKept(t *testing.T) {
	rec := loadRecording(t, "session-resume-context")
	first := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")), Script: resumeScript,
		Prompt: strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
	})
	require.Equal(t, 0, first.Code, first.Stderr)
	elsewhere, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	got := resumeIn(t, first, elsewhere, threadIDs(first)[0], strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "then-01-prompt.txt"))))
	require.Equal(t, 0, got.Code, got.Stderr)

	want := []string{"CTX-startup", "CTX-resume"}
	assert.Equal(t, want, addedContext(t, recordedRollout(t, rec)), "recorded")
	assert.Equal(t, want, addedContext(t, first.rollout(t)), "the mock's")
}
