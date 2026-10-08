package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyOrder is the keys of a JSON object in the order it was written.
func keyOrder(t *testing.T, line string) (keys []string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	_, err := dec.Token()
	require.NoError(t, err)
	for dec.More() {
		k, err := dec.Token()
		require.NoError(t, err)
		keys = append(keys, k.(string))
		var skip json.RawMessage
		require.NoError(t, dec.Decode(&skip))
	}
	return keys
}

// firstLines is the first hook payload line of each event in a log, by event name.
func firstLines(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	out := map[string]string{}
	for _, l := range strings.Split(string(data), "\n") {
		var p struct {
			Event string `json:"hook_event_name"`
		}
		if json.Unmarshal([]byte(l), &p) == nil && p.Event != "" && out[p.Event] == "" {
			out[p.Event] = l
		}
	}
	return out
}

// TestT017_104_ScratchpadDirOnASessionThatHasOne: a session that has a scratchpad tells
// every hook of it (scratchpad_dir, <tmp>/claude-<uid>/<encoded cwd>/<session>/scratchpad,
// after cwd), as the recording nested-session-env shows for an entrypoint claude does not
// know; under the entrypoints a -p run takes by itself (snapshots/runs/scratchpad-dir) it has
// none and no payload names one. No payload carries `effort`, which only a model that takes the
// parameter has (every recording ran haiku).
// sr:proves hook-common-payload/claude
func TestT017_104_ScratchpadDirOnASessionThatHasOne(t *testing.T) {
	recorded := firstLines(t, recordedFile(t, "../../snapshots/runs/nested-session-env/samples/*/payloads.jsonl"))
	run := func(entrypoint string) (map[string]string, string, string) {
		dir := t.TempDir()
		tmp := filepath.Join(dir, "tmp")
		log := filepath.Join(dir, "payloads.log")
		h := payloadLogger(t, dir, "log.sh", log, "")
		settings(t, dir, map[string]string{"SessionStart": h, "PreToolUse": h, "PostToolUse": h, "Stop": h})
		sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"true"}`))
		out, code := runInDir(t, dir, []string{"CLAUDE_CODE_ENTRYPOINT=" + entrypoint, "CLAUDE_CODE_TMPDIR=" + tmp}, "--script", sc,
			"--session-id", "sp-1", "--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
		require.Equal(t, 0, code, out)
		return firstLines(t, log), tmp, dir
	}
	with, tmp, dir := run("decoy")
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	root, err := filepath.EvalSymlinks(filepath.Join(tmp, "claude-"+strconv.Itoa(os.Getuid())))
	require.NoError(t, err)
	want := filepath.Join(root, encode(t, resolved), "sp-1", "scratchpad")
	for _, ev := range []string{"SessionStart", "PreToolUse", "PostToolUse", "Stop"} {
		var p map[string]any
		require.NoError(t, json.Unmarshal([]byte(with[ev]), &p))
		assert.Equal(t, want, p["scratchpad_dir"], "%s is told the scratchpad", ev)
		got, rec := keyOrder(t, with[ev]), keyOrder(t, recorded[ev])
		assert.Equal(t, "cwd", got[2])
		assert.Equal(t, "scratchpad_dir", got[3], "after cwd, as recorded")
		assert.Equal(t, rec[:4], got[:4], "%s: the common fields in the recorded order", ev)
		assert.NotContains(t, with[ev], `"effort"`, "no effort: the model of every recording (haiku) takes none, and the mock has none")
	}
	without, _, _ := run("sdk-cli")
	for ev, line := range without {
		assert.NotContains(t, line, "scratchpad_dir", "%s of a session with no scratchpad", ev)
	}
}
