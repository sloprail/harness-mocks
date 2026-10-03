package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/session-resume-unknown: `codex exec resume <id>` for a
// session no rollout holds, with every hook configured (the start and end of
// the session among them).

// Resuming a session that does not exist fails with a no-rollout message on
// stderr and a non-zero exit, and nothing else happens: no event on stdout, no
// session started, and no hook fires, neither the end of the session nor its
// start (runs/session-resume-unknown). The recording shows no end hook, which
// the statement has; that is the declared deviation.
// sr:proves session-resume-unknown/codex
func TestResumeOfUnknownSessionFailsWithoutAnyHook(t *testing.T) {
	rec := loadRecording(t, "session-resume-unknown")
	args := strings.Fields(readFile(t, filepath.Join(rec.setup, "args")))
	require.Len(t, args, 2, "the recorded run resumes one session id")
	wantStderr := strings.TrimSpace(readFile(t, filepath.Join(rec.sample, "stderr.txt")))
	wantExit := strings.TrimSpace(readFile(t, filepath.Join(rec.sample, "exit.txt")))
	require.Equal(t, "1", wantExit)
	require.Empty(t, readFile(t, filepath.Join(rec.sample, "payloads.jsonl")), "the recording fired a hook")
	require.Empty(t, readFile(t, filepath.Join(rec.sample, "stream.jsonl")), "the recording printed an event")

	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo, home, hookLog := filepath.Join(root, "repo"), filepath.Join(root, "home", ".codex"), filepath.Join(root, "hook.log")
	for _, d := range []string{repo, home} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	require.NoError(t, exec.Command("git", "-C", repo, "init", "-q").Run())
	require.NoError(t, os.WriteFile(filepath.Join(home, "hooks.json"), []byte(readFile(t, filepath.Join(rec.setup, "hooks.json"))), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "hook.sh"), []byte(readFile(t, filepath.Join(rec.setup, "hook.sh"))), 0o755))
	script := filepath.Join(root, "scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(callThenResult), 0o755))

	cmdArgs := append([]string{"exec", "--json", "--skip-git-repo-check", "--script", script, "-m", "mock-model"}, args...)
	cmdArgs = append(cmdArgs, strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))))
	cmd := exec.Command(mockBinary, cmdArgs...)
	cmd.Dir = repo
	cmd.Env = []string{"CODEX_HOME=" + home, "HOOK_LOG=" + hookLog, "PATH=" + os.Getenv("PATH")}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	var exit *exec.ExitError
	require.ErrorAs(t, cmd.Run(), &exit)

	assert.Equal(t, 1, exit.ExitCode())
	assert.Equal(t, wantStderr, strings.TrimSpace(errb.String()))
	assert.Empty(t, out.String())
	assert.NoFileExists(t, hookLog, "a hook fired")
	assert.NoDirExists(t, filepath.Join(home, "sessions"), "a session was started")
}
