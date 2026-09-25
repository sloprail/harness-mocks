// Package toolexec executes Claude Code tool calls in-process during mock runs.
//
// The mock uses a turn-based protocol: when the scenario script emits an
// assistant record containing a tool_use block, the mock executes the tool
// locally and then re-invokes the script so it can emit the next turn based
// on the tool's result.
//
// Only the subset of tools needed for realistic e2e scenario scripts is
// implemented here. Unknown tools return an error result so the script can
// handle them explicitly.
//
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/tools-overview
package toolexec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Result is the output of a tool execution.
type Result struct {
	// Output is the text content returned to the model.
	Output string
	// IsError is true when the tool execution failed and the output is an error message.
	IsError bool
}

// Execute runs the named tool with the given JSON input and returns its result.
// cwd is the working directory for tools that operate on the filesystem.
//
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/tools-overview
func Execute(ctx context.Context, toolName string, input json.RawMessage, cwd string) Result {
	switch toolName {
	case "Bash":
		return executeBash(ctx, input, cwd)
	case "Read":
		return executeRead(input, cwd)
	case "Write":
		return executeWrite(input, cwd)
	case "Edit":
		return executeEdit(input, cwd)
	case "Glob":
		return executeGlob(input, cwd)
	default:
		return Result{
			Output:  fmt.Sprintf("tool %q is not implemented in the mock", toolName),
			IsError: true,
		}
	}
}

// bashInput is the argument shape for the Bash tool.
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
type bashInput struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

func executeBash(ctx context.Context, raw json.RawMessage, cwd string) Result {
	var inp bashInput
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Command == "" {
		return Result{Output: "Bash: missing or invalid 'command' field", IsError: true}
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", inp.Command) //nolint:gosec
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	text := strings.TrimRight(string(out), "\n")
	if err != nil {
		return Result{Output: text + "\n" + err.Error(), IsError: true}
	}
	return Result{Output: text}
}

// readInput is the argument shape for the Read tool.
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
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
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
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
// a10n:docs https://docs.anthropic.com/en/docs/claude-code/tools-reference
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

func resolvePath(p, cwd string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cwd, p)
}
