package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_71_SubagentFilesSurviveCompactionAndResume: a session's sub-agent
// transcripts and sidecars (docs: they persist within their session, and a
// resume of the session reaches them) are untouched by a compaction of the
// session's own transcript and by resuming the session: the same files with
// the same bytes, and a sub-agent of the resumed turn lands beside them.
// sr:proves subagent-transcripts/claude
func TestT017_71_SubagentFilesSurviveCompactionAndResume(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	sub := script(t, dir, "sub")
	first := script(t, dir, "first",
		toolUse("a1", "Agent", `{"prompt":"one","description":"first","script":"`+sub+`"}`),
		`{"type":"compact","summary":"compacted @MARK@","trigger":"manual"}`)
	out, code := runInDir(t, dir, nil, "--script", first, "--session-id", "keep-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)

	main := transcriptPath(t, cfg, dir, "keep-1")
	glob := filepath.Join(main[:len(main)-len(".jsonl")], "subagents", "agent-*")
	before, err := filepath.Glob(glob)
	require.NoError(t, err)
	require.Len(t, before, 2, "the sub-agent's file and its sidecar")
	assert.Contains(t, readString(t, main), "compact_boundary", "the session was compacted after the sub-agent ran")
	bytesBefore := map[string]string{}
	for _, f := range before {
		bytesBefore[f] = readString(t, f)
	}

	second := script(t, dir, "second", toolUse("a2", "Agent", `{"prompt":"two","description":"second","script":"`+sub+`"}`))
	out, code = runInDir(t, dir, nil, "--script", second, "--resume", "keep-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "again")
	require.Equal(t, 0, code, out)
	after, err := filepath.Glob(glob)
	require.NoError(t, err)
	assert.Len(t, after, 4, "the resumed turn's sub-agent sits beside the first one's")
	for f, b := range bytesBefore {
		assert.Equal(t, b, readString(t, f), "%s is untouched by the compaction and the resume", filepath.Base(f))
	}
}

// TestT017_72_OldSubagentTranscriptsAreNotSweptByARun holds the mock to the
// transcript-retention run: a print-mode session started with a session of
// another project 63 days old, its sub-agent file and sidecar beside it, and
// cleanupPeriodDays set to 30 removes none of them (the docs' retention sweep
// is not something the recorded run did; the mock has none either).
// sr:proves subagent-transcripts/claude
func TestT017_72_OldSubagentTranscriptsAreNotSweptByARun(t *testing.T) {
	for _, f := range []string{"oldsess.jsonl", "oldsess/subagents/agent-aaaa.jsonl", "oldsess/subagents/agent-aaaa.meta.json"} {
		recordedFile(t, "../../snapshots/runs/transcript-retention/samples/*/transcript/"+f) // recorded: still there after the run
	}

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	old := filepath.Join(cfg, "projects", "-earlier-project")
	oldFiles := []string{
		filepath.Join(old, "oldsess.jsonl"),
		filepath.Join(old, "oldsess", "subagents", "agent-aaaa.jsonl"),
		filepath.Join(old, "oldsess", "subagents", "agent-aaaa.meta.json"),
	}
	then := time.Now().Add(-63 * 24 * time.Hour)
	for _, f := range oldFiles {
		write(t, f, "{}\n", 0o644)
		require.NoError(t, os.Chtimes(f, then, then))
	}
	write(t, filepath.Join(cfg, "settings.json"), `{"cleanupPeriodDays": 30}`, 0o644)
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s"), "--session-id", "fresh-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	for _, f := range oldFiles {
		assert.FileExists(t, f, "a run does not sweep old transcripts")
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
