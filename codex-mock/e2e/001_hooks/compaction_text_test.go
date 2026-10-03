package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Plain text on a compaction hook's stdout is ignored: the compaction goes
// on, and the hooks that follow it still fire.
// sr:proves manual-compaction/codex
func TestCompactionHookPlainTextIsIgnored(t *testing.T) {
	got := execMock(t, scenario{
		HooksJSON: hooksJSON(`"$(git rev-parse --show-toplevel)"/hook.sh`, "PreCompact", "PostCompact"),
		Files:     map[string]string{"hook.sh": "#!/bin/sh\ncat >>\"$HOOK_LOG\"\necho >>\"$HOOK_LOG\"\necho 'continue: false'\n"},
		Script: `#!/bin/sh
c=$(grep -c '"type":"compacted"' "$A10N_MOCK_SESSION_FILE")
if [ "$c" -eq 0 ]; then printf '%s\n' '{"type":"compact","trigger":"auto"}'; else printf '%s\n' '{"type":"result","subtype":"success","result":"DONE"}'; fi
`,
		Prompt: "compact",
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Equal(t, []string{"PreCompact:auto", "PostCompact:auto"}, hookShape(got.hookLog()))
	assert.Equal(t, 1, compactedRecords(got.rollout(t)))
	assert.Contains(t, got.Stdout, "turn.completed")
}
