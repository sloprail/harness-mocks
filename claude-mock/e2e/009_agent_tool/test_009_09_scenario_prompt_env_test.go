package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT009_09_ScenarioSeesPromptAndHookContext: the scenario script receives the
// user's prompt, unchanged, in A10N_MOCK_PROMPT, and the additional context a
// UserPromptSubmit hook returns in A10N_MOCK_ADDITIONAL_CONTEXT: the context is
// added beside the prompt and never replaces it.
// sr:proves scenario-prompt-env
// sr:proves prompt-context-appended
func TestT009_09_ScenarioSeesPromptAndHookContext(t *testing.T) {
	for _, tc := range []struct{ name, hookOut, wantContext string }{
		{"hook adds context", `{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"ctx from hook"}}`, "ctx from hook"},
		{"hook adds nothing", ``, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			promptLog := filepath.Join(dir, "prompt.log")
			ctxLog := filepath.Join(dir, "ctx.log")
			writeSettings(t, dir, map[string]string{
				"UserPromptSubmit": writeHook(t, dir, "ups.sh", "cat >/dev/null\nprintf '%s' '"+tc.hookOut+"'"),
			})
			script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf %s "$A10N_MOCK_PROMPT" > "`+promptLog+`"
printf %s "$A10N_MOCK_ADDITIONAL_CONTEXT" > "`+ctxLog+`"
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`)
			prompt := "the user's prompt, with  two spaces"
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s-p", "--project-dir", dir, "-p", prompt)
			require.Equal(t, 0, code, "output:\n%s", out)

			got, err := os.ReadFile(promptLog)
			require.NoError(t, err)
			assert.NotEmpty(t, string(got), "the prompt reaches the script")
			gotCtx, err := os.ReadFile(ctxLog)
			require.NoError(t, err)
			assert.Equal(t, tc.wantContext, string(gotCtx), "the hook's context reaches the script beside the prompt")
		})
	}
}
