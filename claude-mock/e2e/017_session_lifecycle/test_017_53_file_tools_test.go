package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const stateNote = " (file state is current in your context — no need to Read it back)"

// TestT017_53_FileToolsAsRecorded replays snapshots/runs/file-tools through the
// binary and holds each call to what the real harness answered: a write says
// whether it created or updated, a read numbers its lines from 1 (an offset and
// a limit select lines; a final newline ends a last, empty line; an empty file
// answers with a warning), an edit says it succeeded, an edit whose string is
// absent or ambiguous is refused as a tool_use_error before any hook fires, a
// read of a missing file is an error that did run (PostToolUseFailure). The
// hooks' payloads carry the structured responses (Write: type, filePath,
// structuredPatch, originalFile; Read: file{startLine, numLines, totalLines};
// Edit: oldString, newString, replaceAll).
// sr:proves file-tools/claude
func TestT017_53_FileToolsAsRecorded(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	h := payloadLogger(t, dir, "log.sh", log, "")
	settings(t, dir, map[string]string{"PreToolUse": h, "PostToolUse": h, "PostToolUseFailure": h})
	cwd, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	f, empty, missing := filepath.Join(dir, "f.txt"), filepath.Join(dir, "empty.txt"), filepath.Join(dir, "missing.txt")
	q := func(s string) string { return fmt.Sprintf("%q", s) }
	calls := []string{
		toolUse("w1", "Write", `{"file_path":`+q(f)+`,"content":"alpha\nbeta\nalpha\n"}`),
		toolUse("r1", "Read", `{"file_path":`+q(f)+`}`),
		toolUse("r2", "Read", `{"file_path":`+q(f)+`,"offset":2,"limit":1}`),
		toolUse("e1", "Edit", `{"file_path":`+q(f)+`,"old_string":"beta","new_string":"BETA"}`),
		toolUse("e2", "Edit", `{"file_path":`+q(f)+`,"old_string":"alpha","new_string":"A"}`),
		toolUse("e3", "Edit", `{"file_path":`+q(f)+`,"old_string":"no-such-text","new_string":"z"}`),
		toolUse("w2", "Write", `{"file_path":`+q(f)+`,"content":"replaced\n"}`),
		toolUse("w3", "Write", `{"file_path":`+q(empty)+`,"content":""}`),
		toolUse("r3", "Read", `{"file_path":`+q(empty)+`}`),
		toolUse("r4", "Read", `{"file_path":`+q(missing)+`}`),
		toolUse("e4", "Edit", `{"file_path":`+q(f)+`,"old_string":"replaced","new_string":"X","replace_all":true}`),
		toolUse("r5", "Read", `{"file_path":`+q(f)+`,"offset":9}`),
		toolUse("r6", "Read", `{"file_path":`+q(dir)+`}`),
	}
	out, code := runInDir(t, dir, nil, "--script", script(t, dir, "s", calls...), "--session-id", "ft-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "ft-1"))
	result := func(id string) (string, bool) {
		b, _ := toolResultOf(t, recs, id+"turn-s-"+string(rune('a'+idx(calls, id))))
		return fmt.Sprint(b["content"]), b["is_error"] == true
	}
	for _, tc := range []struct {
		id, want string
		isErr    bool
	}{
		{"w1", "File created successfully at: " + f + stateNote, false},
		{"r1", "1\talpha\n2\tbeta\n3\talpha\n4\t", false},
		{"r2", "2\tbeta", false},
		{"e1", "The file " + f + " has been updated successfully.", false},
		{"e2", "<tool_use_error>Found 2 matches of the string to replace, but replace_all is false. To replace all occurrences, set replace_all to true. To replace only one occurrence, please provide more context to uniquely identify the instance.\nString: alpha</tool_use_error>", true},
		{"e3", "<tool_use_error>String to replace not found in file.\nString: no-such-text</tool_use_error>", true},
		{"w2", "The file " + f + " has been updated successfully." + stateNote, false},
		{"w3", "File created successfully at: " + empty + stateNote, false},
		{"r3", "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>", false},
		{"r4", "File does not exist. Note: your current working directory is " + cwd + ".", true},
	} {
		got, isErr := result(tc.id)
		assert.Equal(t, tc.want, got, tc.id)
		assert.Equal(t, tc.isErr, isErr, tc.id)
	}
	e4, e4err := result("e4") // replace_all: the wording is unrecorded, the file is what counts
	assert.Contains(t, e4, "The file "+f+" has been updated")
	assert.False(t, e4err)
	r5, _ := result("r5")
	assert.Contains(t, r5, "<system-reminder>", "an offset past the last line answers with a notice")
	assert.Contains(t, r5, "2 lines", "which gives the file's line count")
	r6, r6err := result("r6")
	assert.True(t, r6err, "Read reads files, not directories")
	assert.Contains(t, r6, "directory")
	onDisk, err := os.ReadFile(f)
	require.NoError(t, err)
	assert.Equal(t, "X\n", string(onDisk))

	var seen []string
	resp := map[string]map[string]any{}
	for _, p := range payloads(t, log) {
		id, _ := p["tool_use_id"].(string)
		ev := p["hook_event_name"].(string)
		seen = append(seen, ev+" "+id[:2])
		if r, ok := p["tool_response"].(map[string]any); ok {
			resp[id[:2]] = r
		}
	}
	for _, id := range []string{"e2", "e3"} {
		assert.NotContains(t, strings.Join(seen, "|"), " "+id, "an edit refused as invalid fires no hook")
	}
	assert.Contains(t, seen, "PostToolUseFailure r4", "a read of a missing file ran and failed")
	assert.Contains(t, seen, "PreToolUse r4")
	assert.Equal(t, "create", resp["w1"]["type"])
	assert.Equal(t, []any{}, resp["w1"]["structuredPatch"])
	assert.Nil(t, resp["w1"]["originalFile"])
	assert.Equal(t, "update", resp["w2"]["type"])
	assert.Equal(t, "alpha\nBETA\nalpha\n", resp["w2"]["originalFile"])
	assert.Equal(t, []any{map[string]any{"oldStart": 1.0, "oldLines": 3.0, "newStart": 1.0, "newLines": 1.0,
		"lines": []any{"-alpha", "-BETA", "-alpha", "+replaced"}}}, resp["w2"]["structuredPatch"])
	assert.Equal(t, map[string]any{"filePath": f, "content": "alpha\nbeta\nalpha\n", "numLines": 4.0, "startLine": 1.0, "totalLines": 4.0}, resp["r1"]["file"])
	assert.Equal(t, map[string]any{"filePath": f, "content": "beta", "numLines": 1.0, "startLine": 2.0, "totalLines": 4.0}, resp["r2"]["file"])
	assert.Equal(t, "beta", resp["e1"]["oldString"])
	assert.Equal(t, "BETA", resp["e1"]["newString"])
	assert.Equal(t, false, resp["e1"]["replaceAll"])
	assert.Equal(t, []any{map[string]any{"oldStart": 1.0, "oldLines": 3.0, "newStart": 1.0, "newLines": 3.0,
		"lines": []any{" alpha", "-beta", "+BETA", " alpha"}}}, resp["e1"]["structuredPatch"])
	assert.Equal(t, true, resp["e4"]["replaceAll"])
	assert.Equal(t, map[string]any{"filePath": empty, "content": "", "numLines": 1.0, "startLine": 1.0, "totalLines": 1.0}, resp["r3"]["file"])
}

func idx(calls []string, id string) int {
	for i, c := range calls {
		if strings.Contains(c, `"id":"`+id) {
			return i
		}
	}
	return -1
}

// TestT017_54_GlobListsMatchesOldestFirstUpToAHundred: a glob lists the files
// that match, relative to the directory searched, sorted by modification time
// (recorded: snapshots/runs/file-tools: the older file first), supports **, {a,b}
// and a path, finds nothing for a pattern that matches only directories, and
// stops at a hundred files, saying so in its structured result (docs, Glob tool
// behavior).
// sr:proves file-tools/claude
func TestT017_54_GlobListsMatchesOldestFirstUpToAHundred(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config")
	log := filepath.Join(dir, "payloads.log")
	settings(t, dir, map[string]string{"PostToolUse": payloadLogger(t, dir, "log.sh", log, "")})
	base := time.Now().Add(-time.Hour)
	mk := func(rel string, age int) {
		p := filepath.Join(dir, "tree", rel)
		write(t, p, "x", 0o644)
		require.NoError(t, os.Chtimes(p, base.Add(time.Duration(age)*time.Second), base.Add(time.Duration(age)*time.Second)))
	}
	mk("newer.txt", 20)
	mk("older.txt", 10)
	mk("sub/deep/c.txt", 30)
	mk("sub/b.json", 40)
	mk("sub/c.yaml", 50)
	for i := 0; i < 105; i++ {
		mk(fmt.Sprintf("many/f%03d.log", i), 100+i)
	}
	tree := filepath.Join(dir, "tree")
	q := func(pattern string) string {
		return `{"pattern":"` + pattern + `","path":"` + tree + `"}`
	}
	sc := script(t, dir, "s",
		toolUse("g1", "Glob", q("*.txt")), toolUse("g2", "Glob", q("**/*.txt")), toolUse("g3", "Glob", q("sub/*.{json,yaml}")),
		toolUse("g4", "Glob", q("sub")), toolUse("g5", "Glob", q("many/*.log")),
		toolUse("g6", "Glob", `{"pattern":"tree/*.txt"}`), toolUse("g7", "Glob", `{"pattern":"*.txt","path":"`+tree+`\u0000"}`),
		toolUse("g8", "Glob", `{"pattern":"*.txt\u0000"}`))
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "gl-1",
		"--project-dir", dir, "--config-dir", cfg, "-p", "hello")
	require.Equal(t, 0, code, out)
	recs := readRecs(t, transcriptPath(t, cfg, dir, "gl-1"))
	text := func(i int) string {
		b, _ := toolResultOf(t, recs, "g"+fmt.Sprint(i+1)+"turn-s-"+string(rune('a'+i)))
		return fmt.Sprint(b["content"])
	}
	assert.Equal(t, "older.txt\nnewer.txt", text(0))
	assert.Equal(t, "older.txt\nnewer.txt\nsub/deep/c.txt", text(1))
	assert.Equal(t, "sub/b.json\nsub/c.yaml", text(2))
	assert.Equal(t, "No files found", text(3), "a directory is not a file")
	assert.Len(t, strings.Split(text(4), "\n"), 100)
	assert.Equal(t, "tree/older.txt\ntree/newer.txt", text(5), "with no path, the working directory is searched")
	assert.Contains(t, text(6), "null byte", "a path with a null byte is an error asking for it to be removed")
	assert.Contains(t, text(7), "null byte", "so is a pattern with one")
	resp := map[string]map[string]any{}
	for _, p := range payloads(t, log) {
		resp[p["tool_use_id"].(string)[:2]] = p["tool_response"].(map[string]any)
	}
	assert.Equal(t, []any{"older.txt", "newer.txt"}, resp["g1"]["filenames"])
	assert.EqualValues(t, 2, resp["g1"]["numFiles"])
	assert.Equal(t, false, resp["g1"]["truncated"])
	assert.EqualValues(t, 100, resp["g5"]["numFiles"])
	assert.EqualValues(t, 105, resp["g5"]["totalMatches"])
	assert.Equal(t, true, resp["g5"]["truncated"])
	assert.Equal(t, true, resp["g1"]["countIsComplete"])
}
