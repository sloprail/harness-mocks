package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// grepDir is a project whose files were written a minute apart, oldest first, as the
// recorded run wrote its own (snapshots/runs/grep-tool): a.txt, b.txt, sub/c.txt.
func grepDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	start := time.Now().Add(-time.Hour)
	for i, f := range []struct{ name, content string }{
		{"a.txt", "alpha\nbeta\nalpha beta\n"}, {"b.txt", "gamma\nAlpha\n"}, {"sub/c.txt", "alpha in sub\n"},
	} {
		path := filepath.Join(dir, f.name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(f.content), 0o644))
		at := start.Add(time.Duration(i) * time.Minute)
		require.NoError(t, os.Chtimes(path, at, at))
	}
	return dir
}

// searches numbers the sessions, each call one of its own.
var searches atomic.Int32

// grep runs one Grep call of the given input in dir and returns the run's output.
func grep(t *testing.T, dir, input string) string {
	t.Helper()
	script := filepath.Join(dir, ".scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$A10N_MOCK_SESSION_FILE" ] && grep -q "tool_result" "$A10N_MOCK_SESSION_FILE" 2>/dev/null; then
  printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'
  exit 0
fi
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_1","name":"Grep","input":`+input+`}]}}'
`), 0o755))
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", fmt.Sprintf("grep-%d", searches.Add(1)), "--project-dir", dir, "--config-dir", filepath.Join(dir, ".cfg"), "-p", "go")
	require.Equal(t, 0, code, "output:\n%s", out)
	return out
}

// TestT019_01_FilesWithMatchesIsTheDefault: a search lists the files with a match, newest first,
// under "Found N files"; one that matches nothing says "No files found" (recorded: runs/grep-tool).
// sr:proves grep-tool/claude
func TestT019_01_FilesWithMatchesIsTheDefault(t *testing.T) {
	dir := grepDir(t)
	out := grep(t, dir, `{"pattern":"alpha"}`)
	assert.Contains(t, out, `"content":"Found 2 files\nsub/c.txt\na.txt"`)
	assert.Contains(t, out, `"tool_use_result":{"filenames":["sub/c.txt","a.txt"],"mode":"files_with_matches","numFiles":2,"totalFiles":2}`)
	assert.Contains(t, grep(t, dir, `{"pattern":"no-such-text-zzz"}`), `"content":"No files found"`)
}

// TestT019_02_ContentShowsTheMatchingLines: the content mode lists "file:line:text" in path order,
// "file:text" without line numbers, and a single file's lines without its name; no match is "No
// matches found" (recorded: runs/grep-tool).
// sr:proves grep-tool/claude
func TestT019_02_ContentShowsTheMatchingLines(t *testing.T) {
	dir := grepDir(t)
	out := grep(t, dir, `{"pattern":"alpha","output_mode":"content","-n":true}`)
	assert.Contains(t, out, `"content":"a.txt:1:alpha\na.txt:3:alpha beta\nsub/c.txt:1:alpha in sub"`)
	assert.Contains(t, out, `"numLines":3,"totalLines":3`)
	assert.Contains(t, grep(t, dir, `{"pattern":"alpha","output_mode":"content","-n":false}`), `"content":"a.txt:alpha\na.txt:alpha beta\nsub/c.txt:alpha in sub"`)
	assert.Contains(t, grep(t, dir, `{"pattern":"alpha","output_mode":"content","path":"sub"}`), `"content":"sub/c.txt:1:alpha in sub"`)
	assert.Contains(t, grep(t, dir, `{"pattern":"alpha","output_mode":"content","path":"a.txt"}`), `"content":"1:alpha\n3:alpha beta"`)
	assert.Contains(t, grep(t, dir, `{"pattern":"zzz","output_mode":"content"}`), `"content":"No matches found"`)
}

// TestT019_03_CountTotalsTheMatches: the count mode lists each file's number of matching lines and
// a total across the files; no match counts 0 across 0 files (recorded: runs/grep-tool).
// sr:proves grep-tool/claude
func TestT019_03_CountTotalsTheMatches(t *testing.T) {
	dir := grepDir(t)
	out := grep(t, dir, `{"pattern":"alpha","output_mode":"count"}`)
	assert.Contains(t, out, `"content":"a.txt:2\nsub/c.txt:1\n\nFound 3 total occurrences across 2 files."`)
	assert.Contains(t, out, `"numFiles":2`)
	assert.Contains(t, out, `"numMatches":3`)
	assert.Contains(t, grep(t, dir, `{"pattern":"zzz","output_mode":"count"}`), `"content":"No matches found\n\nFound 0 total occurrences across 0 files."`)
}

// TestT019_04_CaseAndGlobNarrowTheSearch: -i matches without regard to case, and a glob keeps the
// files whose name matches (recorded: runs/grep-tool).
// sr:proves grep-tool/claude
func TestT019_04_CaseAndGlobNarrowTheSearch(t *testing.T) {
	dir := grepDir(t)
	assert.Contains(t, grep(t, dir, `{"pattern":"alpha","-i":true}`), `"content":"Found 3 files\nsub/c.txt\nb.txt\na.txt"`)
	assert.Contains(t, grep(t, dir, `{"pattern":"alpha","glob":"b*"}`), `"content":"No files found"`)
	assert.Contains(t, grep(t, dir, `{"pattern":"alpha","-i":true,"glob":"b*"}`), `"content":"Found 1 file\nb.txt"`)
}

// TestT019_05_AFailedSearchIsAnErrorResult: a pattern that does not parse and a path that is not there
// are error results saying why (recorded: runs/grep-tool).
// sr:proves grep-tool/claude
func TestT019_05_AFailedSearchIsAnErrorResult(t *testing.T) {
	dir := grepDir(t)
	out := grep(t, dir, `{"pattern":"("}`)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, `Search failed — ripgrep rejected the pattern, glob, or file type without searching:\nrg: regex parse error:\n    (?:()\n    ^\nerror: unclosed group`)
	out = grep(t, dir, `{"pattern":"alpha","path":"no-such-dir"}`)
	assert.Contains(t, out, `"is_error":true`)
	assert.Contains(t, out, `Path does not exist: no-such-dir. Note: your current working directory is `)
}

// TestT019_06_AParameterTheMockDoesNotImplementIsRefused: a search that asks for what the mock does
// not model (a file type, a head limit) fails the run (adr/tool-calls-validated).
func TestT019_06_AParameterTheMockDoesNotImplementIsRefused(t *testing.T) {
	dir := grepDir(t)
	script := filepath.Join(dir, ".scenario.sh")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
printf '%s\n' '{"type":"assistant","message":{"role":"assistant","stop_reason":null,"content":[{"type":"tool_use","id":"tu_1","name":"Grep","input":{"pattern":"alpha","head_limit":1}}]}}'
`), 0o755))
	out, code := runInDir(t, dir, nil, "--script", script, "--session-id", "s1", "--project-dir", dir, "--config-dir", filepath.Join(dir, ".cfg"), "-p", "go")
	require.NotZero(t, code, out)
	assert.Contains(t, out, "head_limit")
}
