package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runMock(t *testing.T, dir, configDir, sessionID string, extraArgs ...string) (string, int) {
	t.Helper()
	script := filepath.Join(dir, "s.sh")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"system","subtype":"init","session_id":"s1","tools":[]}'
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))
	}
	args := append([]string{
		"--script", script,
		"--session-id", sessionID,
		"--project-dir", dir,
		"--config-dir", configDir,
		"-p", "go",
	}, extraArgs...)
	return runInDir(t, dir, nil, args...)
}

// TestT008_01_SessionFileCreatedUnderConfigDir: session JSONL appears under configDir/projects/.
// staged:proves session-transcript-file/claude
func TestT008_01_SessionFileCreatedUnderConfigDir(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	_, code := runMock(t, dir, configDir, "sess-1")
	require.Equal(t, 0, code)

	var found []string
	filepath.Walk(configDir, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(p) == ".jsonl" {
			found = append(found, p)
		}
		return nil
	})
	require.Len(t, found, 1, "exactly one session file must exist")
	assert.Contains(t, found[0], "sess-1.jsonl")
}

// TestT008_02_SessionFilePathEncoding: cwd path is encoded with non-alphanumeric → '-'.
func TestT008_02_SessionFilePathEncoding(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	_, code := runMock(t, dir, configDir, "sess-2")
	require.Equal(t, 0, code)

	// Encoded dir must only contain alphanumeric and '-'.
	var projectDirs []string
	projectsRoot := filepath.Join(configDir, "projects")
	entries, err := os.ReadDir(projectsRoot)
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() {
			projectDirs = append(projectDirs, e.Name())
		}
	}
	require.Len(t, projectDirs, 1)
	nonAlphanum := regexp.MustCompile(`[^a-zA-Z0-9\-]`)
	assert.False(t, nonAlphanum.MatchString(projectDirs[0]),
		"encoded project dir must only contain [a-zA-Z0-9-], got: %s", projectDirs[0])
}

// TestT008_03_SessionFileContainsForwardedRecords: the session file contains the
// forwarded trajectory records (a system init here), but NOT the `result` frame —
// real Claude Code never persists the result to the transcript file (verified: 0
// type:"result" records in a real ~/.claude/projects/<proj>/<session>.jsonl). The
// result is a stdout-stream-only frame, so it must appear on stdout and be absent
// from the file.
func TestT008_03_SessionFileContainsForwardedRecords(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	stdout, code := runMock(t, dir, configDir, "sess-3")
	require.Equal(t, 0, code)

	var sessionFile string
	filepath.Walk(configDir, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(p) == ".jsonl" {
			sessionFile = p
		}
		return nil
	})
	require.NotEmpty(t, sessionFile)
	data, err := os.ReadFile(sessionFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"type":"system"`, "forwarded trajectory records are persisted")
	// FIX 2: the result frame is stream-only. It is on stdout…
	assert.Contains(t, stdout, `"type":"result"`, "the result frame must be streamed to stdout")
	// …and NOT in the transcript file, matching real Claude Code.
	assert.NotContains(t, string(data), `"type":"result"`,
		"real Claude Code never persists the result frame to the transcript file")
}

// TestT008_04_SessionFileAppendsAcrossTurns: tool_result from second turn appears in session file.
func TestT008_04_SessionFileAppendsAcrossTurns(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")

	// Script that does one tool_use turn then a result.
	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_1","name":"Bash","input":{"command":"echo appended"}}]}}'
`), 0o755))

	_, code := runInDir(t, dir, nil,
		"--script", script,
		"--session-id", "sess-4",
		"--project-dir", dir,
		"--config-dir", configDir,
		"-p", "go",
	)
	require.Equal(t, 0, code)

	var sessionFile string
	filepath.Walk(configDir, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(p) == ".jsonl" {
			sessionFile = p
		}
		return nil
	})
	require.NotEmpty(t, sessionFile)
	data, err := os.ReadFile(sessionFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"tool_use"`, "assistant tool_use must be in session")
	assert.Contains(t, string(data), `"tool_result"`, "synthesised tool_result must be in session")
	assert.Contains(t, string(data), "appended", "bash output must appear in session")
}

// TestT008_05_CLAUDECONFIGDIREnvOverridesDefault: CLAUDE_CONFIG_DIR env is honoured when no --config-dir flag.
// staged:proves session-transcript-file/claude
func TestT008_05_CLAUDECONFIGDIREnvOverridesDefault(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "env-config")

	script := filepath.Join(dir, "s.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
`), 0o755))

	// Pass CLAUDE_CONFIG_DIR via env, no --config-dir flag.
	_, code := runInDir(t, dir, []string{"CLAUDE_CONFIG_DIR=" + configDir},
		"--script", script,
		"--session-id", "sess-5",
		"--project-dir", dir,
		"-p", "go",
	)
	require.Equal(t, 0, code)

	var found []string
	filepath.Walk(configDir, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() && filepath.Ext(p) == ".jsonl" {
			found = append(found, p)
		}
		return nil
	})
	require.Len(t, found, 1, "session file must be created under CLAUDE_CONFIG_DIR")
}
