package toolexec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// grepInput is the argument shape for the Grep tool: the parameters the mock implements (toolschema.go).
// sr:docs https://code.claude.com/docs/en/tools-reference#grep-tool-behavior
type grepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`
	OutputMode string `json:"output_mode,omitempty"`
	IgnoreCase bool   `json:"-i,omitempty"`
	LineNumber *bool  `json:"-n,omitempty"`
}

// executeGrep answers a Grep the way claude 2.1.285 does (recorded: snapshots/runs/grep-tool): the files
// that match, newest first, the lines that match with their numbers, or the count of each file with a
// total; nothing found is said in words; a pattern that does not parse and a path that is not there are
// errors. The toolUseResult carries the mode and the counts.
//
// sr:provides grep-tool/claude
func executeGrep(raw json.RawMessage, cwd string) Result {
	var inp grepInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Pattern == "" {
		return Result{Output: "Grep: missing or invalid 'pattern' field", IsError: true}
	}
	mode := inp.OutputMode
	if mode == "" {
		mode = tools.GrepFiles
	}
	found, err := tools.Grep(tools.GrepQuery{Pattern: inp.Pattern, Dir: cwd, Path: inp.Path, Glob: inp.Glob, IgnoreCase: inp.IgnoreCase})
	var parse *tools.ParseError
	switch {
	case errors.As(err, &parse):
		return failed("Search failed — ripgrep rejected the pattern, glob, or file type without searching:\n" + parse.Diagnostic)
	case errors.Is(err, tools.ErrNoSuchPath):
		return failed("Path does not exist: " + inp.Path + ". Note: your current working directory is " + cwd + ".")
	case err != nil:
		return failed(err.Error())
	}
	switch mode {
	case tools.GrepContent:
		return grepContent(found, inp.LineNumber == nil || *inp.LineNumber)
	case tools.GrepCount:
		return grepCount(found)
	}
	names := []string{}
	for _, h := range found.ByRecency() {
		names = append(names, h.Name)
	}
	structured := map[string]any{"mode": mode, "filenames": names, "numFiles": len(names), "totalFiles": len(names)}
	if len(names) == 0 {
		return Result{Output: "No files found", ToolUseResult: structured}
	}
	return Result{Output: "Found " + plural(len(names), "file") + "\n" + strings.Join(names, "\n"), ToolUseResult: structured}
}

func grepContent(found tools.GrepResult, numbered bool) Result {
	var lines []string
	for _, h := range found.Hits {
		for i, text := range h.Lines {
			prefix := ""
			if !found.SingleFile {
				prefix = h.Name + ":"
			}
			if numbered {
				prefix += strconv.Itoa(h.Numbers[i]) + ":"
			}
			lines = append(lines, prefix+text)
		}
	}
	content := strings.Join(lines, "\n")
	structured := map[string]any{"mode": tools.GrepContent, "numFiles": 0, "filenames": []string{}, "content": content, "numLines": len(lines), "totalLines": len(lines)}
	if len(lines) == 0 {
		return Result{Output: "No matches found", ToolUseResult: structured}
	}
	return Result{Output: content, ToolUseResult: structured}
}

func grepCount(found tools.GrepResult) Result {
	var lines []string
	total := 0
	for _, h := range found.Hits {
		n := strconv.Itoa(len(h.Numbers))
		if !found.SingleFile {
			n = h.Name + ":" + n
		}
		lines = append(lines, n)
		total += len(h.Numbers)
	}
	content := strings.Join(lines, "\n")
	summary := fmt.Sprintf("Found %s across %s.", plural(total, "total occurrence"), plural(len(found.Hits), "file"))
	structured := map[string]any{"mode": tools.GrepCount, "numFiles": len(found.Hits), "filenames": []string{}, "content": content, "numMatches": total}
	if len(lines) == 0 {
		return Result{Output: "No matches found\n\n" + summary, ToolUseResult: structured}
	}
	return Result{Output: content + "\n\n" + summary, ToolUseResult: structured}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
