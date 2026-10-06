package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
