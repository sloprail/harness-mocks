package toolexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// readInput is the argument shape for the Read tool.
// sr:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
type readInput struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func executeRead(raw json.RawMessage, cwd string) Result {
	var inp readInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.FilePath == "" {
		return Result{Output: "Read: missing or invalid 'file_path' field", IsError: true}
	}

	path := resolvePath(inp.FilePath, cwd)
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}

	lines := strings.Split(string(data), "\n")
	start := 0
	if inp.Offset > 0 {
		start = inp.Offset
	}
	if start >= len(lines) {
		return Result{Output: ""}
	}
	end := len(lines)
	if inp.Limit > 0 && start+inp.Limit < end {
		end = start + inp.Limit
	}
	return Result{Output: strings.Join(lines[start:end], "\n")}
}

// writeInput is the argument shape for the Write tool.
// sr:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
type writeInput struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func executeWrite(raw json.RawMessage, cwd string) Result {
	var inp writeInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.FilePath == "" {
		return Result{Output: "Write: missing or invalid 'file_path' field", IsError: true}
	}

	path := resolvePath(inp.FilePath, cwd)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	if err := os.WriteFile(path, []byte(inp.Content), 0o644); err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("File written successfully to %s", inp.FilePath)}
}

// editInput is the argument shape for the Edit tool.
// sr:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
type editInput struct {
	FilePath  string `json:"file_path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

func executeEdit(raw json.RawMessage, cwd string) Result {
	var inp editInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.FilePath == "" {
		return Result{Output: "Edit: missing or invalid 'file_path' field", IsError: true}
	}

	path := resolvePath(inp.FilePath, cwd)
	data, err := os.ReadFile(path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}
	}

	original := string(data)
	count := strings.Count(original, inp.OldString)
	if count == 0 {
		return Result{Output: fmt.Sprintf("Edit: old_string not found in %s", inp.FilePath), IsError: true}
	}
	if count > 1 {
		return Result{Output: fmt.Sprintf("Edit: old_string appears %d times in %s; must be unique", count, inp.FilePath), IsError: true}
	}

	updated := strings.Replace(original, inp.OldString, inp.NewString, 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return Result{Output: err.Error(), IsError: true}
	}
	return Result{Output: fmt.Sprintf("File %s edited successfully", inp.FilePath)}
}
