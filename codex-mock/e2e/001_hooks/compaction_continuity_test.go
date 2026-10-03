package e2e

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/compaction-transcript-continuity: three shell commands
// under -c model_auto_compact_token_limit=4000, which compacted the session
// after each of them.

// compactedPayloads are the payloads of the `compacted` records of a rollout.
func compactedPayloads(rollout string) (out []map[string]any) {
	for _, r := range jsonLines(rollout) {
		if r["type"] == "compacted" {
			out = append(out, r["payload"].(map[string]any))
		}
	}
	return
}

// checkCompactionChain asserts what the recording shows of every compaction in
// a rollout: the windows chain the compactions (each closes the window the one
// before opened, numbered from 1, all of the same first window), and the
// replacement history holds the prompt as the kept tail of earlier records
// (named by its retained source) and then the summary, an opaque compaction item.
func checkCompactionChain(t *testing.T, rollout, prompt string, atLeast int) {
	t.Helper()
	cs := compactedPayloads(rollout)
	require.GreaterOrEqual(t, len(cs), atLeast)
	previous := cs[0]["first_window_id"]
	for i, c := range cs {
		assert.Equal(t, previous, c["previous_window_id"], "compaction %d closes the window before it", i+1)
		assert.Equal(t, cs[0]["first_window_id"], c["first_window_id"])
		assert.EqualValues(t, i+1, c["window_number"])
		assert.NotEqual(t, previous, c["window_id"])
		previous = c["window_id"]

		history := c["replacement_history"].([]any)
		meta := c["replacement_history_metadata"].([]any)
		require.Len(t, meta, len(history))
		last := history[len(history)-1].(map[string]any)
		assert.Equal(t, "compaction", last["type"], "the summary comes after what is kept")
		assert.NotEmpty(t, last["encrypted_content"])
		var kept bool
		for j, h := range history {
			m := h.(map[string]any)
			src, ok := meta[j].(map[string]any)["retained_source"].(map[string]any)
			if !ok {
				continue
			}
			kept = kept || (m["role"] == "user" && strings.Contains(m["content"].([]any)[0].(map[string]any)["text"].(string), prompt) &&
				src["id"].(map[string]any)["message_id"] == m["id"])
		}
		assert.True(t, kept, "the prompt is kept, named by its retained source")
	}
}

// A compaction leaves, in the rollout, one record that opens a window naming
// the one it closes, and holds the kept user message and then the summary: the
// recording and the mock show the same chain across the compactions of a turn.
// sr:proves compaction-transcript-continuity/codex
func TestCompactionLeavesAChainOfWindowsWithTheKeptTailAndSummary(t *testing.T) {
	rec := loadRecording(t, "compaction-transcript-continuity")
	prompt := strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt")))
	files, err := filepath.Glob(filepath.Join(rec.sample, "transcript", "*.jsonl"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	checkCompactionChain(t, readFile(t, files[0]), prompt, 3)

	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    compactingScript, Prompt: prompt, Env: withCalls(t, rec.calls...),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	checkCompactionChain(t, got.rollout(t), prompt, 3)
}
