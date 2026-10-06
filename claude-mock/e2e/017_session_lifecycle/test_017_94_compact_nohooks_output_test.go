package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const commandOutput = "<local-command-stdout>Compacted </local-command-stdout>"

// A manual /compact with no hooks writes, and streams, exactly the command's
// output the recorded run did (runs/compact-nohooks): the stream's frame and the
// transcript's record each carry the one string.
// sr:proves manual-compaction/claude
func TestT017_94_ManualCompactionWithoutHooksOutputIsAsRecorded(t *testing.T) {
	recorded, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/compact-nohooks/samples/*/stream.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(recorded), `"content":"`+commandOutput+`"`, "recorded")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`),
		`{"type":"compact","summary":"the summary @MARK@","trigger":"manual"}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-nh", "--project-dir", dir, "--config-dir", cfg,
		"--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, 1, strings.Count(out, commandOutput), "one frame: the command's output")
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-nh"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(raw), commandOutput), "and one transcript record")
}
