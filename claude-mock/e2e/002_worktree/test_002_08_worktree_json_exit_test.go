package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A worktree hook's non-zero exit fails the operation no matter what its JSON
// says: valid JSON that would decide on another event's exit 1 does not save
// a WorktreeCreate or a WorktreeRemove.
// sr:docs https://code.claude.com/docs/en/hooks#exit-code-2-behavior-per-event
// sr:proves hook-exit-code-semantics/claude
func TestT002_08_WorktreeHookJSONDoesNotOverrideNonZeroExit(t *testing.T) {
	for _, tc := range []struct{ event, record string }{
		{"WorktreeCreate", "worktree_create"},
		{"WorktreeRemove", "worktree_remove"},
	} {
		t.Run(tc.event, func(t *testing.T) {
			dir := t.TempDir()
			hook := writeScript(t, dir, "json.sh", `#!/bin/sh
cat >/dev/null
printf '%s' '{"continue": true, "systemMessage": "all fine"}'
echo "json-but-exit-1" >&2
exit 1
`)
			claudeDir := filepath.Join(dir, ".claude")
			require.NoError(t, os.MkdirAll(claudeDir, 0o755))
			s := `{"hooks":{"` + tc.event + `":[{"matcher":"*","hooks":[{"type":"command","command":"` + hook + `"}]}]}}`
			require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(s), 0o644))
			script := writeScript(t, dir, "s.sh", `#!/bin/sh
printf '%s\n' '{"type":"`+tc.record+`","worktree_name":"feat/json"}'
printf '%s\n' '{"type":"result","subtype":"success","result":"ok","is_error":false}'
`)
			out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "-p", "go")
			assert.NotEqual(t, 0, code, "the JSON does not rescue a non-zero exit")
			assert.Contains(t, out, "claude-mock: "+tc.event+" hook blocked")
			assert.Contains(t, out, "json-but-exit-1")
			assert.NotContains(t, out, `"result":"ok"`)
		})
	}
}
