package e2e

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolatedRun runs an orchestrator dispatching one isolated sub-agent under the
// project's settings, and returns the mock's output and the directory the
// sub-agent ran in ("" when it never ran).
func isolatedRun(t *testing.T, dir, done string) (out string, code int, subPwd string) {
	t.Helper()
	pwdLog := filepath.Join(dir, "sub-pwd.log")
	sub := writeScript(t, dir, "sub.sh", "#!/bin/sh\npwd >> "+pwdLog+"\nprintf '%s\\n' '{\"type\":\"result\",\"subtype\":\"success\",\"result\":\"sub done\",\"is_error\":false}'\n")
	out, code = runInDir(t, dir, nil, "--script", writeScript(t, dir, "orch.sh", agentCallThen(sub, done)),
		"--session-id", "wt-path", "--project-dir", dir, "-p", "go")
	b, _ := os.ReadFile(pwdLog)
	return out, code, strings.TrimSpace(string(b))
}

func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return r
}

// TestT009_24_WorktreeCreateHttpHookReturnsThePathInItsJson: an HTTP
// WorktreeCreate hook cannot print, so it returns the directory it made as
// hookSpecificOutput.worktreePath (hooks#worktreecreate-output), and the
// sub-agent runs there.
// sr:proves worktree-hooks/claude
func TestT009_24_WorktreeCreateHttpHookReturnsThePathInItsJson(t *testing.T) {
	dir := t.TempDir()
	made := filepath.Join(dir, "http-made")
	require.NoError(t, os.MkdirAll(made, 0o755))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"hookSpecificOutput":{"hookEventName":"WorktreeCreate","worktreePath":%q}}`, made)
	}))
	defer srv.Close()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"WorktreeCreate":[{"hooks":[{"type":"http","url":"`+srv.URL+`"}]}]}}`), 0o644))
	out, code, pwd := isolatedRun(t, dir, "agentId:")
	require.Equal(t, 0, code, out)
	assert.Equal(t, realPath(t, made), pwd)
}

// TestT009_24b_WorktreeCreateHookPathReading: the path is the last non-empty
// stdout line with terminal escapes stripped, a relative one is taken from the
// session's directory with its `.` and `..` segments collapsed, and one that is
// not a directory that can be entered fails the creation with an error naming it.
// sr:proves worktree-hooks/claude
func TestT009_24b_WorktreeCreateHookPathReading(t *testing.T) {
	t.Run("ansi escapes are stripped", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "ansi-made"), 0o755))
		hook := writeHook(t, dir, "create.sh", `cat >/dev/null; printf '\033[1;32mready\033[0m\n\033[0m\033[32mansi-made\033[0m\n\n'`)
		writeSettings(t, dir, map[string]string{"WorktreeCreate": hook})
		out, code, pwd := isolatedRun(t, dir, "agentId:")
		require.Equal(t, 0, code, out)
		assert.Equal(t, realPath(t, filepath.Join(dir, "ansi-made")), pwd)
	})
	t.Run("relative dot segments are collapsed", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "target"), 0o755))
		hook := writeHook(t, dir, "create.sh", `cat >/dev/null; echo "./a/../a/./target/../target"`)
		writeSettings(t, dir, map[string]string{"WorktreeCreate": hook})
		out, code, pwd := isolatedRun(t, dir, "agentId:")
		require.Equal(t, 0, code, out)
		assert.Equal(t, realPath(t, filepath.Join(dir, "a", "target")), pwd)
	})
	t.Run("a path that is not a directory fails the creation", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "plain-file")
		require.NoError(t, os.WriteFile(file, nil, 0o644))
		for name, p := range map[string]string{"file": file, "missing": filepath.Join(dir, "nowhere")} {
			hook := writeHook(t, dir, "create.sh", "cat >/dev/null; echo "+p)
			writeSettings(t, dir, map[string]string{"WorktreeCreate": hook})
			out, code, pwd := isolatedRun(t, dir, "could not create the worktree: the worktree hook returned "+p)
			require.Equal(t, 0, code, name+": "+out)
			assert.Empty(t, pwd, name+": the sub-agent never ran")
		}
	})
}
