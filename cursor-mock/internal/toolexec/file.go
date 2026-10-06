package toolexec

import (
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/sloprail/harness-mocks/internal/tools"
)

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
	if _, ok := c.Args["limit"]; ok { // recorded only against a file that is not there (runs/compaction-transcript-continuity)
		msg := "cursor-mock: Read of a file that exists with a limit is not modeled"
		return failed(msg, msg)
	}
	total := strings.Count(content, "\n") + 1
	body := map[string]any{
		"content": content, "isEmpty": content == "", "exceededLimit": false, "totalLines": total, "fileSize": len(content),
		"path": filepath.Clean(path), "readRange": map[string]any{"startLine": 1, "endLine": total}, // the result names the file resolved; the hooks, as given (recorded: runs/no-add-dir-access)
		"relatedCursorRulePaths": []string{}, "relatedCursorRules": []string{},
	}
	if len(content) > BigBytes { // too big for the frame: it names the content by an id instead
		delete(body, "content")
		body["contentBlobId"] = contentBlobID(content)
	}
	return Result{
		Frame: map[string]any{"success": body},
		Read:  &ReadFile{path, content},
		ToolOutput: jsonString(struct {
			FilePath      string `json:"file_path"`
			ContentLength int    `json:"content_length"`
		}{path, len(utf16.Encode([]rune(content)))}), // content_length counts characters the way the harness does, in UTF-16 units (recorded: runs/schedule-wakeup-ask, a file with non-ASCII text)
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
	if c.Replace != nil {
		var err error
		if content, err = replaced(c, dir); err != nil {
			return failed(err.Error(), err.Error())
		}
	}
	old, existed, err := tools.WriteFile(path, content)
	if err != nil {
		return failed(err.Error(), err.Error())
	}
	before, after := trimShared(old, content)
	frame := map[string]any{
		"path": path, "linesAdded": lines(content), "linesRemoved": lines(old),
		"afterFullFileContent": content, "message": "Wrote contents to " + path,
	}
	if c.Replace != nil { // an edit says what the file was and what changed (recorded: runs/file-tools)
		frame["beforeFullFileContent"], frame["diffString"] = old, diff(path, old, content, true)
		frame["message"] = "The file " + path + " has been updated."
	} else if !existed { // a new file's diff is against nothing
		frame["diffString"] = diff(path, "", content, false)
	}
	return Result{
		Frame: map[string]any{"success": frame},
		ToolOutput: jsonString(struct {
			FilePath string `json:"file_path"`
			Success  bool   `json:"success"`
		}{path, true}),
		Edits: []Edit{{OldString: before, NewString: after}},
	}
}
