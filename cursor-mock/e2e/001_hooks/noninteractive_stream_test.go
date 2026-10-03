package e2e

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// kindsOf lists "type/subtype" for each frame, splitting off the kinds the
// caller names so that what is set aside is asserted, not silently dropped.
func kindsOf(frames []map[string]any, setAside ...string) (kept, aside []string) {
	for _, f := range frames {
		k, _ := f["type"].(string)
		if s, _ := f["subtype"].(string); s != "" {
			k += "/" + s
		}
		isAside := false
		for _, a := range setAside {
			isAside = isAside || a == k
		}
		if isAside {
			aside = append(aside, k)
		} else {
			kept = append(kept, k)
		}
	}
	return kept, aside
}

// TestRecordedStreamCarriesThinkingFramesTheMockOmits: the recorded streams
// of runs/pretool-refusal and runs/noninteractive-no-force carry thinking
// frames (delta frames with text, then a completed frame, each with a session
// id and timestamp) and assistant text frames, which are the model's own. The
// mock has no model and emits neither; every other frame of the recorded
// sequence is in its replay, in the same order (declared as a modeled-surface
// deviation).
// sr:proves noninteractive-run/cursor
func TestRecordedStreamCarriesThinkingFramesTheMockOmits(t *testing.T) {
	modelOnly := []string{"thinking/delta", "thinking/completed", "assistant"}
	for _, run := range []string{"pretool-refusal", "noninteractive-no-force"} {
		got, _ := replay(t, run)
		recorded := readJSONL(t, filepath.Join(newestSample(t, run), "stream.jsonl"))
		printed := readJSONLText(t, got.stdout)

		open := false
		for _, f := range recorded {
			if f["type"] != "thinking" {
				require.False(t, open, run)
				continue
			}
			require.NotEmpty(t, f["session_id"], run)
			require.NotNil(t, f["timestamp_ms"], run)
			if f["subtype"] == "delta" {
				require.NotEmpty(t, f["text"], run)
				open = true
			} else {
				require.Equal(t, "completed", f["subtype"], run)
				require.True(t, open, run)
				open = false
			}
		}

		want, wantAside := kindsOf(recorded, modelOnly...)
		have, haveAside := kindsOf(printed, modelOnly...)
		require.Contains(t, wantAside, "thinking/delta", run)
		require.Contains(t, wantAside, "thinking/completed", run)
		require.Empty(t, haveAside, run)
		require.Equal(t, want, have, run)
		require.Equal(t, "system/init", have[0], run)
		require.Equal(t, "user", have[1], run)
		require.Equal(t, "result/success", have[len(have)-1], run)
	}
}
