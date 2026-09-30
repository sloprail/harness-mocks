package toolexec

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// globInput is the argument shape for the Glob tool.
type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func executeGlob(raw json.RawMessage, cwd string) Result {
	var inp globInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Pattern == "" {
		return Result{Output: "Glob: missing or invalid 'pattern' field", IsError: true}
	}

	base := cwd
	if inp.Path != "" {
		base = resolvePath(inp.Path, cwd)
	}

	matches, err := filepath.Glob(filepath.Join(base, inp.Pattern))
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	if len(matches) == 0 {
		return Result{Output: "No files found"}
	}
	return Result{Output: strings.Join(matches, "\n")}
}
