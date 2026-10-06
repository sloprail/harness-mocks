package e2e

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
)

// A resume of an unknown session puts "No conversation found with session ID:
// <id>" on stderr and the error result frame on stdout, each on its own stream,
// and exits 1 (runs/resume-unknown: stderr.txt, stream.jsonl, exit.txt).
// sr:proves session-resume-unknown/claude
func TestT001_13_AnUnknownResumeKeepsItsMessageOnStderrAndItsFrameOnStdout(t *testing.T) {
	run := filepath.Join("..", "..", "snapshots", "runs", "resume-unknown", "samples", "*")
	recErr, err := os.ReadFile(recordedFile(t, filepath.Join(run, "stderr.txt")))
	require.NoError(t, err)
	require.Contains(t, string(recErr), "No conversation found with session ID: ")
	recOut, err := os.ReadFile(recordedFile(t, filepath.Join(run, "stream.jsonl")))
	require.NoError(t, err)
	want := lastResult(t, string(recOut))

	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho RAN\n"), 0o755))
	const id = "00000000-0000-4000-8000-0000000000ff"
	stdout, stderr, code := runSplit(t, dir, nil, "--script", script, "--resume", id, "--project-dir", dir, "--output-format", "stream-json", "-p", "hello")
	assert.Equal(t, 1, code)
	assert.Equal(t, "No conversation found with session ID: "+id, strings.TrimSpace(stderr), "the message, on stderr alone")
	assert.NotContains(t, stdout, "No conversation found with session ID: "+id+"\n", "not as a line of stdout")
	got := lastResult(t, stdout)
	for _, k := range []string{"subtype", "is_error", "num_turns"} {
		assert.Equal(t, want[k], got[k], k)
	}
	assert.Equal(t, id, got["session_id"])
	for k := range got {
		assert.Contains(t, want, k, "the mock's field is one the recorded frame has")
	}
}

// A flag the mock does not know is reported on stderr before the run starts, in claude's
// words, with nothing on stdout and exit status 1 (runs/invalid-flag: stderr.txt, stream.jsonl,
// exit.txt); --bare, which the mock refuses, ends the same way.
// sr:proves noninteractive-run/claude
func TestT001_16_AnInvalidFlagFailsOnStderrBeforeAnyFrame(t *testing.T) {
	run := filepath.Join("..", "..", "snapshots", "runs", "invalid-flag", "samples", "*")
	recErr, err := os.ReadFile(recordedFile(t, filepath.Join(run, "stderr.txt")))
	require.NoError(t, err)
	recOut, err := os.ReadFile(recordedFile(t, filepath.Join(run, "stream.jsonl")))
	require.NoError(t, err)
	require.Empty(t, recOut, "recorded: nothing on stdout")
	dir := t.TempDir()
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho RAN\n"), 0o755))
	stdout, stderr, code := runSplit(t, dir, nil, "--script", script, "--session-id", "if-1", "--project-dir", dir, "--output-format", "stream-json", "--verbose", "--no-such-flag", "-p", "hello")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout, "no frame comes first")
	assert.Equal(t, strings.TrimSpace(string(recErr)), strings.TrimSpace(stderr), "claude's words, on stderr")
	stdout, stderr, code = runSplit(t, dir, nil, "--script", script, "--session-id", "if-2", "--project-dir", dir, "--output-format", "stream-json", "--bare", "-p", "hello")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "--bare is not implemented")
}

// An unknown session's SessionEnd hook runs before the error result frame is streamed, and that
// frame is all of stdout: no init frame and no SessionStart hook frame precede it (runs/resume-unknown:
// events.jsonl lists the hook, then the result; stream.jsonl holds the result alone).
// sr:proves session-resume-unknown/claude
func TestT001_17_AnUnknownResumeEndsItsSessionBeforeTheResultFrame(t *testing.T) {
	run := filepath.Join("..", "..", "snapshots", "runs", "resume-unknown", "samples", "*")
	events, err := os.ReadFile(recordedFile(t, filepath.Join(run, "events.jsonl")))
	require.NoError(t, err)
	recorded := strings.Index(string(events), `"hook":"SessionEnd"`) >= 0 && strings.Index(string(events), `"hook":"SessionEnd"`) < strings.Index(string(events), `"event":"stream"`)
	require.True(t, recorded, "recorded: the hook, then the result")
	recOut, err := os.ReadFile(recordedFile(t, filepath.Join(run, "stream.jsonl")))
	require.NoError(t, err)
	require.Len(t, nonEmptyLines(string(recOut)), 1, "recorded: the result alone")

	dir := t.TempDir()
	stamp := filepath.Join(dir, "hook-ended")
	hook := filepath.Join(dir, "end.sh")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\nsleep 0.5\n: > "+stamp+"\n"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".claude", "settings.json"),
		[]byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"`+hook+`"}]}],"SessionEnd":[{"hooks":[{"type":"command","command":"`+hook+`"}]}]}}`), 0o644))
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho RAN\n"), 0o755))
	cmd := exec.Command(e2etest.MockBinaryPath, "--script", script, "--resume", "00000000-0000-4000-8000-0000000000ff", "--project-dir", dir,
		"--output-format", "stream-json", "-p", "hello")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+filepath.Join(dir, "config"), "CLAUDE_CODE_TMPDIR="+filepath.Join(dir, "tmp"))
	pipe, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	var lines []string
	var arrived time.Time
	sc := bufio.NewScanner(pipe)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if arrived.IsZero() {
			arrived = time.Now()
		}
	}
	_ = cmd.Wait()
	require.Len(t, lines, 1, "the result frame is all of stdout")
	assert.Contains(t, lines[0], `"type":"result"`)
	info, err := os.Stat(stamp)
	require.NoError(t, err, "the SessionEnd hook ran")
	assert.True(t, info.ModTime().Before(arrived), "the hook finished before the result frame reached stdout")
}
