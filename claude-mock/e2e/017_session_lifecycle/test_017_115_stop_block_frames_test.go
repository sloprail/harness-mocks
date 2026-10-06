package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stopFrames are the stream frames a blocking Stop hook leaves, as kind:text-or-key in order: the
// synthetic feedback message to the agent, the stop-hook-error notification, and at the cap the
// informational frame and the stop-hook-block-cap notification.
func stopFrames(frames []map[string]any) (out []string) {
	for _, f := range frames {
		switch {
		case f["type"] == "user" && f["isSynthetic"] == true:
			msg := f["message"].(map[string]any)["content"].([]any)[0].(map[string]any)
			out = append(out, "feedback:"+msg["text"].(string))
		case f["subtype"] == "notification":
			out = append(out, "notification:"+f["key"].(string))
		case f["subtype"] == "informational":
			out = append(out, "informational")
		}
	}
	return out
}

// TestT017_115_ABlockingStopStreamsItsFeedbackAndNotices: each Stop block streams the feedback the
// agent reads as a synthetic user frame; the stop-hook-error notification is shown once however many
// blocks follow; at the cap an informational frame and a stop-hook-block-cap notification end the turn,
// and the run counts that as a turn (a cap of N blocks fires Stop N+1 times; recording cap: 9 feedbacks, one error notice, then the cap).
// sr:proves stop-block-cap/claude
// sr:proves stop-block-continuation/claude
func TestT017_115_ABlockingStopStreamsItsFeedbackAndNotices(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/cap/samples/*/stream.jsonl"))
	require.NoError(t, err)
	recorded := stopFrames(streamFrames(t, string(data)))
	require.Equal(t, "notification:stop-hook-error", recorded[1])
	require.Equal(t, []string{"informational", "notification:stop-hook-block-cap"}, recorded[len(recorded)-2:])

	dir := t.TempDir()
	h := write(t, filepath.Join(dir, "stop.sh"), "#!/bin/sh\ncat >/dev/null\necho '{\"decision\":\"block\",\"reason\":\"KEEP GOING\"}'\n", 0o755)
	settings(t, dir, map[string]string{"Stop": h})
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
echo '{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"DONE"}]}}'
echo '{"type":"result","subtype":"success","result":"DONE"}'
`, 0o755)
	out, code := runInDir(t, dir, []string{"CLAUDE_CODE_STOP_HOOK_BLOCK_CAP=2"}, "--script", sc, "--session-id", "sb-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "go")
	require.Equal(t, 0, code, out)
	got := stopFrames(streamFrames(t, out))
	assert.Equal(t, []string{
		"feedback:Stop hook feedback:\nKEEP GOING", "notification:stop-hook-error", "feedback:Stop hook feedback:\nKEEP GOING",
		"feedback:Stop hook feedback:\nKEEP GOING", "informational", "notification:stop-hook-block-cap",
	}, got)
	assert.Equal(t, recorded[0], got[0], "the feedback text as recorded")
}
