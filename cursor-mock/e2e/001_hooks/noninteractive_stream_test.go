package e2e

import (
	"os"
	"os/exec"
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
// id and timestamp) and assistant text frames. The mock calls no model, so it
// emits no thinking frames, whatever the script says; it prints an assistant
// frame only for text the script gives it (the positive control below), and
// the replay scripts carry none. Every other frame of the recorded sequence
// is in its replay, in the same order.
// sr:proves noninteractive-run/cursor
func TestRecordedStreamCarriesThinkingFramesTheMockOmits(t *testing.T) {
	thinking := []string{"thinking/delta", "thinking/completed"}
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

		// the recording has thinking and assistant frames; the replay has no
		// thinking frame, and no assistant frame because its scripts say nothing
		want, wantAside := kindsOf(recorded, append([]string{"assistant"}, thinking...)...)
		have, haveAside := kindsOf(printed, append([]string{"assistant"}, thinking...)...)
		require.Contains(t, wantAside, "thinking/delta", run)
		require.Contains(t, wantAside, "thinking/completed", run)
		require.Contains(t, wantAside, "assistant", run)
		require.Empty(t, haveAside, run)
		require.Equal(t, want, have, run)
		require.Equal(t, "system/init", have[0], run)
		require.Equal(t, "user", have[1], run)
		require.Equal(t, "result/success", have[len(have)-1], run)
	}
}

// TestAScriptsTextIsAnAssistantFrameAndNoThinkingFrameFollows: the positive
// control of the test above. A script that says something gets an assistant
// text frame in the stream (so the replay's lack of them is the scripts', not a
// mock that cannot print them), and still no thinking frame, which is the
// model's own.
// sr:proves noninteractive-run/cursor
func TestAScriptsTextIsAnAssistantFrameAndNoThinkingFrameFollows(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s\\n' '{\"type\":\"assistant\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"on it\"}]}}'\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"DONE\"}'\n"), 0o755))
	cmd := exec.Command(binary, "-p", "--force", "--trust", "--output-format", "stream-json", "--script", script, "go")
	cmd.Dir, cmd.Env = dir, []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	out, err := cmd.Output()
	require.NoError(t, err, string(out))

	kept, aside := kindsOf(readJSONLText(t, string(out)), "assistant")
	require.Equal(t, []string{"assistant"}, aside, string(out))
	require.Equal(t, []string{"system/init", "user", "result/success"}, kept, string(out))
	require.Contains(t, string(out), `"text":"on it"`)
}
