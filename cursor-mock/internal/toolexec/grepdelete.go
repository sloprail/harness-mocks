package toolexec

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// grep runs a Grep call: the files of the workspace are searched for the
// pattern, and the result lists, per file ("./<path>"), the lines that match.
// What it reports to postToolUse is the pattern and that it succeeded
// (recorded: runs/hook-matchers-grep-delete). A search with no match, a search
// of one path and the other output modes are not recorded: the call fails
// rather than guess.
//
// sr:provides hook-matcher-filter/cursor
func grep(c Call, dir string) Result {
	if len(c.Unmodeled) > 0 {
		msg := "cursor-mock: Grep takes only a pattern; " + strings.Join(c.Unmodeled, ", ") + " is not modeled"
		return failed(msg, msg)
	}
	pattern := c.str("pattern")
	re, err := regexp.Compile(pattern)
	if err != nil {
		return failed(err.Error(), err.Error())
	}
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".cursor") {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	var found []any
	total := 0
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var matches []any
		for i, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
			if re.MatchString(l) {
				matches = append(matches, map[string]any{"lineNumber": i + 1, "content": l, "contentTruncated": false, "isContextLine": false})
			}
		}
		if len(matches) > 0 {
			rel, _ := filepath.Rel(dir, p)
			found = append(found, map[string]any{"file": "./" + filepath.ToSlash(rel), "matches": matches})
			total += len(matches)
		}
	}
	if total == 0 {
		return failed("cursor-mock: a Grep with no match is not modeled", "cursor-mock: a Grep with no match is not modeled")
	}
	return Result{
		Frame: map[string]any{"success": map[string]any{
			"pattern": pattern, "path": "", "outputMode": "content",
			"workspaceResults": map[string]any{dir: map[string]any{"content": map[string]any{
				"matches": found, "totalLines": total, "totalMatchedLines": total, "clientTruncated": false, "ripgrepTruncated": false}}},
		}},
		ToolOutput: jsonString(struct {
			Pattern string `json:"pattern"`
			Success bool   `json:"success"`
		}{pattern, true}),
	}
}

// deleteFile runs a Delete call: the file is removed, and the result says what
// it was (its size, as a string, and its content). postToolUse reports the
// path and that it was deleted (recorded: runs/hook-matchers-grep-delete). A
// file that is not there is not recorded: the call fails rather than guess.
//
// sr:provides hook-matcher-filter/cursor
func deleteFile(c Call, dir string) Result {
	path := c.Path(dir)
	old, err := os.ReadFile(path)
	if err != nil {
		return failed("cursor-mock: a Delete of a file that cannot be read is not modeled: "+err.Error(), "cursor-mock: a Delete of a file that cannot be read is not modeled")
	}
	if err := os.Remove(path); err != nil {
		return failed(err.Error(), err.Error())
	}
	return Result{
		Frame: map[string]any{"success": map[string]any{
			"path": path, "deletedFile": path, "fileSize": strconv.Itoa(len(old)), "prevContent": string(old)}},
		ToolOutput: jsonString(struct {
			FilePath string `json:"file_path"`
			Deleted  bool   `json:"deleted"`
		}{path, true}),
	}
}

// ran times a tool's call: postToolUse reports its duration, a small positive
// fraction of a millisecond or more.
func ran(f func() Result) Result {
	start := time.Now()
	r := f()
	r.Took = max(time.Since(start), time.Microsecond)
	return r
}
