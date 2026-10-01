package toolexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// readInput is the argument shape for the Read tool.
// sr:docs https://code.claude.com/docs/en/tools-reference#read-tool-behavior
type readInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// executeRead answers a Read the way claude 2.1.285 does (recorded:
// snapshots/runs/file-tools, tool-errors): the lines numbered, a tab after each
// number, from line offset for limit lines; the toolUseResult says which
// lines they are.
//
// sr:provides file-tools/claude
func executeRead(raw json.RawMessage, cwd string) Result {
	var inp readInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.FilePath == "" {
		return Result{Output: "Read: missing or invalid 'file_path' field", IsError: true}
	}
	path := resolvePath(inp.FilePath, cwd)
	if st, serr := os.Stat(path); serr == nil && st.IsDir() { // Read reads files, not directories (docs, Read tool behavior)
		return failed("EISDIR: illegal operation on a directory, read")
	}
	content, err := tools.ReadFile(path)
	if errors.Is(err, tools.ErrNotFound) {
		return failed("File does not exist. Note: your current working directory is " + cwd + ".")
	}
	if err != nil {
		return failed(err.Error())
	}
	v := tools.Read(content, inp.Offset, inp.Limit)
	structured := map[string]any{"type": "text", "file": map[string]any{
		"filePath": path, "content": v.Content, "numLines": v.NumLines, "startLine": v.StartLine, "totalLines": v.TotalLines,
	}}
	switch v.OutcomeOf(content) {
	case tools.ReadEmpty:
		return Result{Output: "<system-reminder>Warning: the file exists but the contents are empty.</system-reminder>", ToolUseResult: structured}
	case tools.ReadPastEnd:
		return Result{Output: fmt.Sprintf("<system-reminder>Warning: the file exists but is shorter than the provided offset (%d). The file has %d lines.</system-reminder>", v.StartLine, v.TotalLines), ToolUseResult: structured}
	}
	lines := tools.Lines(v.Content)
	for i := range lines {
		lines[i] = strconv.Itoa(v.StartLine+i) + "\t" + lines[i]
	}
	return Result{Output: strings.Join(lines, "\n"), ToolUseResult: structured}
}
