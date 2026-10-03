package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_07l_ABlockedCompactionLetsTheSessionGoOn: blocking a compaction,
// automatic or manual, skips it and the conversation continues uncompacted
// (hooks#precompact): the tool call the scenario makes right after the compact
// record, in the same step, runs and is recorded.
// sr:proves manual-compaction/claude
func TestT017_07l_ABlockedCompactionLetsTheSessionGoOn(t *testing.T) {
	for _, trigger := range []string{"manual", "auto"} {
		t.Run(trigger, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, "config")
			block := write(t, filepath.Join(dir, "block.sh"), "#!/bin/sh\ncat >/dev/null\necho blocked 1>&2\nexit 2\n", 0o755)
			compactSettings(t, dir, map[string][2]string{"PreCompact": {"*", block}})
			sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
if grep -q 'after-block-call' "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"done"}]}}'
  echo '{"type":"result","subtype":"success","result":"done"}'
  exit 0
fi
cat <<'JSONL'
{"type":"compact","summary":"x","trigger":"`+trigger+`"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"after-block-call","name":"Bash","input":{"command":"echo AFTER-BLOCK"}}]}}
JSONL
`, 0o755)
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-l",
				"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
			require.Equal(t, 0, code, out)
			raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-l"))
			require.NoError(t, err)
			assert.NotContains(t, string(raw), "compact_boundary", "nothing was compacted")
			assert.Contains(t, string(raw), "AFTER-BLOCK", "the tool call after the blocked compaction ran")
		})
	}
}
