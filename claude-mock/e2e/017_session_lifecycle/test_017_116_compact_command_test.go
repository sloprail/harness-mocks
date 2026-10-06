package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_116_CompactIsAHarnessCommand: /compact is carried out by the harness itself: no
// UserPromptSubmit sees it and no Stop follows it, its result frame says local_command "compact"
// (and has no api_error_status or terminal_reason), and the summary streams as a synthetic user frame
// then the command's own output (recording compact, step 2).
// sr:proves manual-compaction/claude
func TestT017_116_CompactIsAHarnessCommand(t *testing.T) {
	data, err := os.ReadFile(recordedFile(t, "../../snapshots/runs/compact/samples/*/stream.jsonl"))
	require.NoError(t, err)
	frames := streamFrames(t, string(data))
	recResult := frames[len(frames)-1]
	require.Equal(t, "compact", recResult["local_command"])
	require.NotContains(t, recResult, "api_error_status")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"UserPromptSubmit": h, "Stop": h, "PreCompact": h, "PostCompact": h})
	first := script(t, dir, "a", toolUse("b1", "Bash", `{"command":"true"}`))
	out, code := runInDir(t, dir, nil, "--script", first, "--session-id", "cc-1", "--project-dir", dir, "--config-dir", cfg, "-p", "go")
	require.Equal(t, 0, code, out)
	before := len(payloads(t, log))
	sc := script(t, dir, "c", `{"type":"compact","summary":"the summary @MARK@","trigger":"manual"}`)
	out, code = runInDir(t, dir, nil, "--script", sc, "--resume", "cc-1", "--project-dir", dir, "--config-dir", cfg,
		"--output-format", "stream-json", "-p", "/compact")
	require.Equal(t, 0, code, out)
	var events []string
	for _, p := range payloads(t, log)[before:] {
		events = append(events, p["hook_event_name"].(string))
	}
	assert.Equal(t, []string{"PreCompact", "PostCompact"}, events, "no UserPromptSubmit and no Stop for a command of the harness")
	got := streamFrames(t, out)
	result := got[len(got)-1]
	assert.Equal(t, "compact", result["local_command"])
	assert.NotContains(t, result, "api_error_status")
	assert.NotContains(t, result, "terminal_reason")
}
