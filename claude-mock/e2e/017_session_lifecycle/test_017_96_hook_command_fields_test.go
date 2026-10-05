package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runRecordedHooks runs the mock on a recorded run's own setup (its settings and
// hook script) with the Bash call the run's prompt names, and returns the lines
// the hooks logged and the recorded ones.
func runRecordedHooks(t *testing.T, run, session string) (got, want []string) {
	t.Helper()
	setup := filepath.Join("..", "..", "snapshots", "runs", run, "setup")
	dir := t.TempDir()
	for _, f := range []string{"hook.sh", "settings.json"} {
		body, err := os.ReadFile(filepath.Join(setup, f))
		require.NoError(t, err)
		dst := filepath.Join(dir, f)
		if f == "settings.json" {
			dst = filepath.Join(dir, ".claude", "settings.json")
		}
		write(t, dst, string(body), 0o755)
	}
	log := filepath.Join(dir, "hooks.log")
	sc := script(t, dir, "s", toolUse("b1", "Bash", `{"command":"echo HOOKFIELDS"}`))
	out, code := runInDir(t, dir, []string{"HOOK_LOG=" + log}, "--script", sc, "--session-id", session, "--project-dir", dir,
		"--config-dir", filepath.Join(dir, "config"), "-p", "go")
	require.Equal(t, 0, code, out)
	raw, err := os.ReadFile(log)
	require.NoError(t, err)
	rec, err := os.ReadFile(recordedFile(t, filepath.Join("..", "..", "snapshots", "runs", run, "samples", "*", "payloads.jsonl")))
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(raw)), "\n"), strings.Split(strings.TrimSpace(string(rec)), "\n")
}

// A command hook with `args` runs its program directly with those arguments, each
// one an argument ("two words" one of them), not through a shell (recorded:
// runs/hook-args, run from its own settings and hook script).
// sr:proves hook-command-handler/claude
func TestT017_96_AHookWithArgsRunsItsProgramWithThem(t *testing.T) {
	got, want := runRecordedHooks(t, "hook-args", "args-1")
	assert.Equal(t, []string{`{"argc":2,"argv1":"one","argv2":"two words"}`}, want, "recorded")
	assert.Equal(t, want, got)
}

// A hook's `shell` field changes nothing the recording shows: both hooks of
// runs/hook-shell, with and without it, ran under /bin/sh, as the mock runs
// every command line.
// sr:proves hook-command-handler/claude
func TestT017_96_AHookShellFieldChangesNothing(t *testing.T) {
	got, want := runRecordedHooks(t, "hook-shell", "shell-1")
	assert.Equal(t, []string{`{"hook":"with-shell-bash","zero":"/bin/sh"}`, `{"hook":"default","zero":"/bin/sh"}`}, want, "recorded")
	assert.Equal(t, want, got)
}
