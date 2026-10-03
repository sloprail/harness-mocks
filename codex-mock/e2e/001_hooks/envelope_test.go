package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/session-transcript-file: a one-turn exec session whose
// rollout file the sample keeps under transcript/.

// A rollout's bookkeeping is its first record's: session id, start time,
// working directory, how the run was launched, version and git branch; every
// record carries a timestamp, and the later ones carry none of the rest. Both
// the recorded rollout and the mock's have this shape (runs/session-transcript-file).
// sr:proves transcript-record-envelope/codex
func TestRolloutBookkeepingIsTheFirstRecordsAndEveryRecordIsTimestamped(t *testing.T) {
	rec := loadRecording(t, "session-transcript-file")
	files, err := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	recorded, err := os.ReadFile(files[0])
	require.NoError(t, err)
	got := replay(t, rec)
	require.Equal(t, 0, got.Code, got.Stderr)

	for _, p := range []struct {
		name    string
		records []map[string]any
	}{{"recording", jsonLines(string(recorded))}, {"mock", jsonLines(got.rollout(t))}} {
		require.Greater(t, len(p.records), 1, p.name)
		for i, r := range p.records {
			assert.NotEmpty(t, r["timestamp"], "%s: record %d is timestamped", p.name, i)
			if i > 0 {
				for _, k := range []string{"session_id", "cwd", "git", "cli_version", "source"} {
					assert.NotContains(t, r, k, "%s: record %d does not repeat the bookkeeping", p.name, i)
				}
			}
		}
		first := p.records[0]
		assert.Equal(t, "session_meta", first["type"], p.name)
		meta, _ := first["payload"].(map[string]any)
		for _, k := range []string{"session_id", "timestamp", "cwd", "originator", "source", "cli_version"} {
			assert.NotEmpty(t, meta[k], "%s: session_meta carries %s", p.name, k)
		}
		assert.Equal(t, "exec", meta["source"], "%s: a non-interactive run says so in the session record", p.name)
		git, _ := meta["git"].(map[string]any)
		assert.NotEmpty(t, git["branch"], "%s: session_meta names the git branch", p.name)

		for _, r := range p.records {
			payload, _ := r["payload"].(map[string]any)
			if payload["role"] == "user" {
				assert.NotContains(t, payload, "source", "%s: the prompt record itself is not marked", p.name)
			}
		}
	}
}
