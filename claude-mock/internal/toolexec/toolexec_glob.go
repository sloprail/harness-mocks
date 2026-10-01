package toolexec

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/tools"
)

// globInput is the argument shape for the Glob tool.
// sr:docs https://code.claude.com/docs/en/tools-reference#glob-tool-behavior
type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

// executeGlob lists the files that match, relative to the directory searched,
// the oldest modification first, at most a hundred (recorded:
// snapshots/runs/file-tools); the toolUseResult says how many matched and
// whether the list was cut.
//
// sr:provides file-tools/claude
func executeGlob(raw json.RawMessage, cwd string) Result {
	var inp globInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Pattern == "" {
		return Result{Output: "Glob: missing or invalid 'pattern' field", IsError: true}
	}
	if strings.ContainsRune(inp.Pattern, 0) || strings.ContainsRune(inp.Path, 0) {
		return failed("Glob: the pattern and the path must not contain a null byte; remove it")
	}
	base := cwd
	if inp.Path != "" {
		base = resolvePath(inp.Path, cwd)
	}
	started := time.Now()
	found, err := tools.Glob(base, inp.Pattern, tools.GlobLimit)
	if err != nil {
		return failed(err.Error())
	}
	names := found.Names
	if names == nil {
		names = []string{}
	}
	structured := map[string]any{
		"filenames": names, "durationMs": time.Since(started).Milliseconds(), "numFiles": len(names),
		"truncated": found.Truncated, "totalMatches": found.Total, "countIsComplete": true,
	}
	if len(names) == 0 {
		return Result{Output: "No files found", ToolUseResult: structured}
	}
	return Result{Output: strings.Join(names, "\n"), ToolUseResult: structured}
}
