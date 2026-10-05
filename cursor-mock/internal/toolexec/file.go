package toolexec

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// lines is how many lines a text has: a final line without a newline counts.
func lines(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// Edit is one change a write made to a file.
type Edit struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// timed runs a file tool's call and times it: postToolUse reports its duration
// in milliseconds, a small positive fraction (recorded: runs/file-tools).
func timed(c Call, dir string) Result {
	start := time.Now()
	var r Result
	if c.Kind == "readToolCall" {
		r = read(c, dir)
	} else {
		r = write(c, dir)
	}
	r.Took = max(time.Since(start), time.Microsecond)
	return r
}

func failed(message, frameMessage string) Result {
	return Result{Failed: true, ErrorMessage: message, Frame: map[string]any{"error": map[string]any{"errorMessage": frameMessage}}}
}

// read runs a Read call: the file's content is the result. A file that does
// not exist fails the call: the failure hook's error_message is "File not
// found: <path>" (recorded: runs/tool-failure).
//
// A read that succeeds also reports the file to beforeReadFile (Result.Read);
// a failed one does not (recorded: runs/file-tools, runs/tool-failure).
//
// sr:provides file-tools/cursor
// sr:docs https://cursor.com/docs/hooks#beforereadfile
func read(c Call, dir string) Result {
	path := c.Path(dir)
	content, err := tools.ReadFile(path)
	if errors.Is(err, tools.ErrNotFound) {
		return failed("File not found: "+path, "File not found")
	}
	if err != nil {
		return failed(err.Error(), err.Error())
	}
	total := strings.Count(content, "\n") + 1
	return Result{
		Frame: map[string]any{"success": map[string]any{
			"content": content, "isEmpty": content == "", "exceededLimit": false, "totalLines": total, "fileSize": len(content),
			"path": filepath.Clean(path), "readRange": map[string]any{"startLine": 1, "endLine": total}, // the result names the file resolved; the hooks, as given (recorded: runs/no-add-dir-access)
			"relatedCursorRulePaths": []string{}, "relatedCursorRules": []string{},
		}},
		Read: &ReadFile{path, content},
		ToolOutput: jsonString(struct {
			FilePath      string `json:"file_path"`
			ContentLength int    `json:"content_length"`
		}{path, len(content)}),
	}
}

// write runs a Write (edit) call: the file is created, or replaced whole, with
// the call's content. What afterFileEdit reports as the edit is the change with
// the text the old and new contents share, at the start and the end, left out
// (recorded: runs/file-tools).
//
// sr:provides file-tools/cursor
// sr:docs https://cursor.com/docs/hooks#afterfileedit
func write(c Call, dir string) Result {
	path, content := c.Path(dir), c.str("streamContent")
	old, _, err := tools.WriteFile(path, content)
	if err != nil {
		return failed(err.Error(), err.Error())
	}
	before, after := trimShared(old, content)
	return Result{
		Frame: map[string]any{"success": map[string]any{
			"path": path, "linesAdded": lines(content), "linesRemoved": lines(old),
			"afterFullFileContent": content, "message": "Wrote contents to " + path,
		}},
		ToolOutput: jsonString(struct {
			FilePath string `json:"file_path"`
			Success  bool   `json:"success"`
		}{path, true}),
		Edits: []Edit{{OldString: before, NewString: after}},
	}
}

// trimShared drops the longest common prefix, then the longest common suffix
// of what is left, from both texts.
func trimShared(a, b string) (string, string) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	a, b = a[p:], b[p:]
	s := 0
	for s < len(a) && s < len(b) && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	return a[:len(a)-s], b[:len(b)-s]
}
