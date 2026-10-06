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

// firstFrames are the "type/subtype" of the first n frames of a stream.
func firstFrames(stream string, n int) (out []string) {
	for _, l := range strings.Split(stream, "\n") {
		var f map[string]any
		if json.Unmarshal([]byte(l), &f) != nil || f["type"] == nil {
			continue
		}
		s, _ := f["subtype"].(string)
		out = append(out, f["type"].(string)+"/"+s)
		if len(out) == n {
			break
		}
	}
	return
}

// With a SessionStart hook configured a run opens its stream with the hook's
// started and response frames, ahead of everything the script says (the recorded
// streams of stops, bashfail and run-failure: hook_started, hook_response, then
// init, which the mock's script supplies itself).
// sr:proves noninteractive-run/claude
func TestT001_12_TheStreamOpensWithTheSessionStartHookFrames(t *testing.T) {
	for _, run := range []string{"stops", "bashfail", "run-failure"} {
		raw, err := os.ReadFile(recordedFile(t, filepath.Join("..", "..", "snapshots", "runs", run, "samples", "*", "stream.jsonl")))
		require.NoError(t, err)
		assert.Equal(t, []string{"system/hook_started", "system/hook_response"}, firstFrames(string(raw), 2), "recorded "+run)
	}

	dir := t.TempDir()
	hook := filepath.Join(dir, "hook.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"command","command":"`+hook+`"}]}]}}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
echo '{"type":"system","subtype":"init","session_id":"s","tools":[]}'
echo '{"type":"result","subtype":"success","result":"x"}'
`), 0o755))
	out, code := runInDirWithEnv(t, dir, nil, "--script", script, "--session-id", "open-1", "--project-dir", dir, "--output-format", "stream-json", "-p", "go")
	require.Equal(t, 0, code, out)
	assert.Equal(t, []string{"system/hook_started", "system/hook_response", "system/init"}, firstFrames(out, 3))
}

// recordedFile is the one file matching glob.
func recordedFile(t *testing.T, glob string) string {
	t.Helper()
	m, err := filepath.Glob(glob)
	require.NoError(t, err)
	require.Len(t, m, 1, glob)
	return m[0]
}
