package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_49_CommandHookRunsInTheEventsDirectoryWithItsPayload: a command hook
// is a shell line run in the working directory of the event (a sub-agent's
// isolated worktree is T009_03's), and is told the project root as
// CLAUDE_PROJECT_DIR; it reads the event's payload as JSON on
// its standard input, its common fields included (docs, Command hook fields,
// Reference scripts by path, Common input fields; the recorded runs reference
// their hook as "$CLAUDE_PROJECT_DIR"/hook.sh).
// sr:proves hook-command-handler/claude
func TestT017_49_CommandHookRunsInTheEventsDirectoryWithItsPayload(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	proj := filepath.Join(dir, "proj")
	require.NoError(t, os.MkdirAll(proj, 0o755))
	out1 := filepath.Join(dir, "seen.txt")
	write(t, filepath.Join(proj, "hook.sh"), `#!/bin/sh
{ echo "PROJECT=$CLAUDE_PROJECT_DIR"; echo "PWD=$(pwd -P)"; echo "SHELL_LINE_ARG=$1"; cat; echo; } > `+out1+`
`, 0o755)
	write(t, filepath.Join(proj, ".claude", "settings.json"),
		`{"hooks":{"UserPromptSubmit":[{"hooks":[{"type":"command","command":"\"$CLAUDE_PROJECT_DIR\"/hook.sh ARG && true"}]}]}}`, 0o644)
	sc := script(t, proj, "s")
	cmd := []string{"--script", sc, "--session-id", "cmd-1", "--project-dir", proj, "--config-dir", cfg, "-p", "the prompt"}
	out, code := runInDir(t, proj, nil, cmd...)
	require.Equal(t, 0, code, out)

	data, err := os.ReadFile(out1)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.Len(t, lines, 4, string(data))
	wantProj, _ := filepath.EvalSymlinks(proj)
	gotProj, _ := filepath.EvalSymlinks(strings.TrimPrefix(lines[0], "PROJECT="))
	assert.Equal(t, wantProj, gotProj, "CLAUDE_PROJECT_DIR is the project root")
	assert.Equal(t, "PWD="+wantProj, lines[1], "the hook runs in the event's working directory")
	assert.Equal(t, "SHELL_LINE_ARG=ARG", lines[2], "the command is a shell line: quoted path, arguments and && work")
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[3]), &payload), "the payload is JSON on stdin")
	assert.Equal(t, "UserPromptSubmit", payload["hook_event_name"])
	assert.Equal(t, "cmd-1", payload["session_id"])
	assert.Equal(t, "the prompt", payload["prompt"])
	assert.Contains(t, payload, "transcript_path")
	gotCwd, _ := filepath.EvalSymlinks(payload["cwd"].(string))
	assert.Equal(t, wantProj, gotCwd)
}

// TestT017_50_SessionEndTimeouts: a session-end hook shares a short budget
// (1.5 seconds) unless it sets a timeout of its own, which is honoured: a hook
// with no timeout that works for three seconds is cut off, one with a timeout of
// four finishes (docs, Common fields: "SessionEnd hooks share a 1.5-second
// budget; if your settings set a longer per-hook timeout, Claude Code raises the
// budget to match").
// sr:proves hook-timeout/claude
func TestT017_50_SessionEndTimeouts(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	hook := func(name, marker string) string {
		return write(t, filepath.Join(dir, name), "#!/bin/sh\ncat >/dev/null\nsleep 3\n: > "+filepath.Join(dir, marker)+"\n", 0o755)
	}
	short, long := hook("short.sh", "short-finished"), hook("long.sh", "long-finished")
	write(t, filepath.Join(dir, ".claude", "settings.json"),
		`{"hooks":{"SessionEnd":[{"hooks":[{"type":"command","command":"`+short+`"},{"type":"command","command":"`+long+`","timeout":4}]}]}}`, 0o644)
	start := time.Now()
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "se-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	took := time.Since(start)
	_, errShort := os.Stat(filepath.Join(dir, "short-finished"))
	_, errLong := os.Stat(filepath.Join(dir, "long-finished"))
	assert.True(t, os.IsNotExist(errShort), "a hook with no timeout of its own is cut off at the shared budget")
	assert.NoError(t, errLong, "a hook with a timeout of its own gets it")
	assert.Greater(t, took, 3*time.Second, "the run waited for the hook that had the time")
}
