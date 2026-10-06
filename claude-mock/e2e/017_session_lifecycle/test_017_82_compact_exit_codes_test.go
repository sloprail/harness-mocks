package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The exit codes of the compaction hooks (hooks, "Exit code 2 behavior per
// event"): PreCompact blocks only on exit 2, so any other non-zero exit is a
// non-blocking error and the compaction happens; PostCompact cannot affect the
// compaction, even with exit 2.
func compactWith(t *testing.T, pre, post string) (compacted bool, postFired bool) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "post.log")
	hooks := map[string][2]string{}
	if pre != "" {
		hooks["PreCompact"] = [2]string{"*", write(t, filepath.Join(dir, "pre.sh"), "#!/bin/sh\ncat >/dev/null\n"+pre+"\n", 0o755)}
	}
	if post != "" {
		hooks["PostCompact"] = [2]string{"*", payloadLogger(t, dir, "post.sh", log, post)}
	}
	compactSettings(t, dir, hooks)
	compactRun(t, dir, cfg, "cmp-x", "manual")
	raw, err := os.ReadFile(transcriptPath(t, cfg, dir, "cmp-x"))
	require.NoError(t, err)
	_, statErr := os.Stat(log)
	return strings.Contains(string(raw), "compact_boundary"), statErr == nil
}

// sr:proves manual-compaction/claude
func TestT017_82_PreCompactNonBlockingExitLetsTheCompactionHappen(t *testing.T) {
	compacted, post := compactWith(t, "echo oops >&2; exit 1", "exit 0")
	assert.True(t, compacted, "exit 1 is a non-blocking error: the compaction happens")
	assert.True(t, post, "and PostCompact fires")
}

// sr:proves manual-compaction/claude
func TestT017_82_PreCompactExit2BlocksTheCompaction(t *testing.T) {
	compacted, post := compactWith(t, "echo no >&2; exit 2", "exit 0")
	assert.False(t, compacted)
	assert.False(t, post, "PostCompact fires only when the compaction happens")
}

// sr:proves manual-compaction/claude
func TestT017_82_PostCompactFailureDoesNotAffectTheCompaction(t *testing.T) {
	for _, post := range []string{"echo late >&2; exit 2", "echo late >&2; exit 1"} {
		compacted, fired := compactWith(t, "", post)
		assert.True(t, compacted, post)
		assert.True(t, fired, post)
	}
}

// hookFrameNames are the hook_name of every hook_started frame of a stream.
func hookFrameNames(t *testing.T, stream string) []string {
	t.Helper()
	var names []string
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) == nil && f["type"] == "system" && f["subtype"] == "hook_started" {
			names = append(names, f["hook_name"].(string))
		}
	}
	return names
}

// A compaction's PreCompact and PostCompact hooks stream no hook frames: the
// hooks that stream around a compaction are the compacted session's own
// SessionStart:compact, whose frames come after the "compacting" status, as
// recorded in runs/compact (which configures all of them).
// sr:proves manual-compaction/claude
func TestT017_82_CompactionStreamsOnlyTheCompactedSessionStartFrames(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/compact/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := hookFrameNames(t, string(raw))
	for _, n := range want {
		assert.True(t, strings.HasPrefix(n, "SessionStart:"), "recorded: only SessionStart frames, got %s", n)
	}
	assert.Contains(t, want, "SessionStart:compact")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	h := payloadLogger(t, dir, "log.sh", filepath.Join(dir, "p.log"), "")
	compactSettings(t, dir, map[string][2]string{"SessionStart": {"*", h}, "PreCompact": {"*", h}, "PostCompact": {"*", h}})
	sc := script(t, dir, "s", `{"type":"compact","summary":"x @MARK@","trigger":"manual"}`)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "cmp-frames", "--project-dir", dir, "--config-dir", cfg,
		"--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, []string{"SessionStart:startup", "SessionStart:compact"}, hookFrameNames(t, out))
}
