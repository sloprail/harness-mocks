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
	checkAcrossCompactions(t, readFile(t, files[0]), jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))), prompt)

	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		Files:     map[string]string{"hook.sh": readFile(t, filepath.Join(rec.setup, "hook.sh"))},
		Script:    compactingScript, Prompt: prompt, Env: withCalls(t, rec.calls...),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	checkCompactionChain(t, got.rollout(t), prompt, 3)
	checkAcrossCompactions(t, got.rollout(t), got.hookLog(), prompt)
}

// checkAcrossCompactions asserts what the recording shows of the rollout and the hooks across compactions:
// the file only grows (the first tool output, written before the first compaction, is still in it after the
// last, with the later ones), the session, the turn and the transcript path in every hook payload are the
// same before and after, the compacted record's retained context holds the prompt of that turn and its
// resume metadata names it as the turn last started, and the turn's context and the thread's settings follow
// each compacted record.
func checkAcrossCompactions(t *testing.T, rollout string, payloads []map[string]any, prompt string) {
	t.Helper()
	require.NotEmpty(t, payloads)
	for _, p := range payloads {
		for _, k := range []string{"session_id", "turn_id", "transcript_path"} {
			assert.Equal(t, payloads[0][k], p[k], "%s stays the same across the compactions", k)
		}
	}
	records := jsonLines(rollout)
	firstCompacted, firstOutput, outputs := -1, -1, 0
	for i, r := range records {
		p, _ := r["payload"].(map[string]any)
		if ty, _ := p["type"].(string); strings.HasSuffix(ty, "_output") {
			outputs++
			if firstOutput < 0 {
				firstOutput = i
			}
		}
		if r["type"] != "compacted" {
			continue
		}
		if firstCompacted < 0 {
			firstCompacted = i
		}
		next := records[i+1:]
		require.GreaterOrEqual(t, len(next), 2)
		assert.Equal(t, "turn_context", next[0]["type"], "the turn's context follows a compaction")
		settings, _ := next[1]["payload"].(map[string]any)
		assert.Equal(t, "thread_settings_applied", settings["type"], "then the thread's settings")
		assert.Equal(t, payloads[0]["turn_id"], next[0]["payload"].(map[string]any)["turn_id"])
		retained := p["retained_context"].(map[string]any)["user_messages"].([]any)
		require.Len(t, retained, 1)
		assert.Contains(t, retained[0].(map[string]any)["text"], prompt)
		assert.Equal(t, payloads[0]["turn_id"], retained[0].(map[string]any)["turn_id"])
		assert.Equal(t, payloads[0]["turn_id"], p["resume_metadata"].(map[string]any)["last_started_turn_id"])
		assert.NotEmpty(t, p["compaction_response_id"])
	}
	require.GreaterOrEqual(t, firstCompacted, 0)
	assert.Equal(t, 3, outputs, "the three tool outputs are all still in the file")
	assert.Less(t, firstOutput, firstCompacted, "the first output, written before the first compaction, is still there")
}
