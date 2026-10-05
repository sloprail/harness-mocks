package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two recordings of a run's start outside a repository, and the one of
// --output-schema: Codex refuses to run outside a repository unless told
// (--skip-git-repo-check), and does not refuse when it bypasses approvals and
// the sandbox. (The mock refuses --output-schema: it implements none of it,
// see unimplemented_refused_test.go.)

// execIn runs the mock with args in dir, with a fresh CODEX_HOME, and returns
// what it left.
func execIn(t *testing.T, dir string, args ...string) result {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	r := result{Repo: dir, Home: filepath.Join(root, "home", ".codex")}
	require.NoError(t, os.MkdirAll(r.Home, 0o755))
	script := filepath.Join(root, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(callThenResult), 0o755))
	cmd := exec.Command(mockBinary, append([]string{"exec", "--script", script, "-m", "mock-model"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(withCalls(t), "CODEX_HOME="+r.Home, "PATH="+os.Getenv("PATH"))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit)
		r.Code = exit.ExitCode()
	}
	r.Stdout, r.Stderr = out.String(), errb.String()
	return r
}

// shapes are the stream's frames as the recordings' events.jsonl keep them:
// the type and, for an item, its type.
func shapes(stream []map[string]any) (out []string) {
	for _, e := range stream {
		s := e["type"].(string)
		if item, ok := e["item"].(map[string]any); ok {
			s += " " + item["type"].(string)
		}
		out = append(out, s)
	}
	return
}

func recordedShapes(t *testing.T, rec recording) (out []string) {
	for _, e := range jsonLines(readFile(t, filepath.Join(rec.sample, "events.jsonl"))) {
		s := e["type"].(string)
		if sub, ok := e["subtype"].(string); ok {
			s += " " + sub
		}
		out = append(out, s)
	}
	return
}

func recordedExit(t *testing.T, rec recording) int {
	n, err := strconv.Atoi(strings.TrimSpace(readFile(t, filepath.Join(rec.sample, "exit.txt"))))
	require.NoError(t, err)
	return n
}

// Outside a repository, without --skip-git-repo-check, Codex prints that the
// directory is not trusted and exits 1 having streamed nothing and started no
// session (runs/noninteractive-run-git-check-refused); with the flag it runs.
// sr:proves noninteractive-run/codex
func TestRunOutsideARepositoryIsRefusedUnlessTold(t *testing.T) {
	rec := loadRecording(t, "noninteractive-run-git-check-refused")
	require.Equal(t, 1, recordedExit(t, rec))
	require.Empty(t, readFile(t, filepath.Join(rec.sample, "stream.jsonl")), "recorded: nothing on stdout")
	rollouts, _ := filepath.Glob(filepath.Join(rec.sample, "transcript", "*"))
	require.Empty(t, rollouts, "recorded: no session")
	lines := strings.Split(strings.TrimSpace(readFile(t, filepath.Join(rec.sample, "stderr.txt"))), "\n")
	want := lines[len(lines)-1]
	require.Equal(t, "Not inside a trusted directory and --skip-git-repo-check was not specified.", want)

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	got := execIn(t, dir, "--json", "--dangerously-bypass-hook-trust", "go")
	assert.Equal(t, recordedExit(t, rec), got.Code)
	assert.Empty(t, got.Stdout)
	assert.Equal(t, want, strings.TrimSpace(got.Stderr))
	sessions, _ := filepath.Glob(filepath.Join(got.Home, "sessions", "*", "*", "*", "*"))
	assert.Empty(t, sessions, "no session was started")

	ok := execIn(t, dir, "--json", "--skip-git-repo-check", "go")
	require.Equal(t, 0, ok.Code, ok.Stderr)
	assert.Contains(t, shapes(ok.stream()), "turn.completed")
}

// Inside a repository a run needs no flag: every recording but the refused one
// ran in one (the flag was given there, so that is not proved by them).
// sr:proves noninteractive-run/codex
func TestRunInsideARepositoryNeedsNoFlag(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, exec.Command("git", "-C", dir, "init", "-q").Run())
	got := execIn(t, dir, "--json", "go")
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.Contains(t, shapes(got.stream()), "turn.completed")
}

// With --dangerously-bypass-approvals-and-sandbox Codex runs outside a
// repository without --skip-git-repo-check and streams as usual
// (runs/noninteractive-run-no-git-check).
// sr:proves noninteractive-run/codex
func TestBypassingTheSandboxRunsOutsideARepository(t *testing.T) {
	rec := loadRecording(t, "noninteractive-run-no-git-check")
	require.Equal(t, 0, recordedExit(t, rec))
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	got := execIn(t, dir, "--json", "--dangerously-bypass-approvals-and-sandbox", "--dangerously-bypass-hook-trust",
		strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))))
	require.Equal(t, recordedExit(t, rec), got.Code, got.Stderr)
	assert.Equal(t, recordedShapes(t, rec), shapes(got.stream()))
}
