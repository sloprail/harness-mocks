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

// framesAfterResult are the "type/subtype/status" of the frames that follow the
// last result frame of a stream.
func framesAfterResult(stream string) (after []string) {
	var seen bool
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil || f["type"] == nil {
			continue
		}
		if f["type"] == "result" {
			seen, after = true, nil
			continue
		}
		if seen {
			s, _ := f["subtype"].(string)
			status, _ := f["status"].(string)
			if p, ok := f["patch"].(map[string]any); ok {
				status, _ = p["status"].(string)
			}
			if tasks, ok := f["tasks"].([]any); ok && len(tasks) == 0 {
				status = "no tasks"
			}
			after = append(after, f["type"].(string)+"/"+s+"/"+status)
		}
	}
	return
}

// A background command killed at the end of the run is reported after the
// result frame, in the order the recorded run streamed it (runs/bgbash): the
// task list emptied, the task updated as stopped, the notification.
// sr:proves background-bash-reaped-at-exit/claude
func TestT017_95_AKilledBackgroundCommandIsReportedAfterTheResult(t *testing.T) {
	raw, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/bgbash/samples/*/stream.jsonl"))
	require.NoError(t, err)
	want := framesAfterResult(string(raw))
	require.NotEmpty(t, want)

	dir := t.TempDir()
	sc := write(t, filepath.Join(dir, "s.sh"), `#!/bin/sh
if ! grep -q '"tool_result"' "$A10N_MOCK_SESSION_FILE"; then
cat <<'JSONL'
`+strings.Replace(toolUse("bg", "Bash", `{"command":"sleep 9","description":"late","run_in_background":true}`), "@MARK@", "", 1)+`
JSONL
exit 0
fi
echo '{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED"}]}}'
echo '{"type":"result","subtype":"success","result":"LAUNCHED"}'
`, 0o755)
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "bgk-1", "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "--output-format", "stream-json", "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Equal(t, want, framesAfterResult(out))
}
