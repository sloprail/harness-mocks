// Package toolexec executes the Cursor tools the mock models (Shell, Read,
// Write): the results Cursor reports for them, and what its hooks are told.
package toolexec

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// Call is a tool call as the scenario script's tool_call frame names it: the
// tool_call object's kind key (shellToolCall, readToolCall, editToolCall) and
// its args.
type Call struct {
	Kind string
	Args map[string]any
}

// Result is what a tool call came to.
type Result struct {
	Outcome corehooks.ToolOutcome
	// Frame is the result object of the call's completed stream frame.
	Frame map[string]any
	// ToolOutput is the call's result as postToolUse reports it, a JSON string.
	ToolOutput string
	// ErrorMessage is what postToolUseFailure reports when the call failed.
	ErrorMessage string
	// Output is what a shell command printed, for afterShellExecution.
	Output string
	// Edits are the changes a file write made, for afterFileEdit.
	Edits []Edit
	Took  time.Duration
}

// Edit is one change a write made to a file.
type Edit struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

// Name is the tool's name in hooks: Shell, Read or Write; "" for a tool the
// mock does not model.
func (c Call) Name() string {
	return map[string]string{"shellToolCall": "Shell", "readToolCall": "Read", "editToolCall": "Write"}[c.Kind]
}

func (c Call) str(key string) string { s, _ := c.Args[key].(string); return s }

// Path is the file a Read or Write call names, resolved against dir.
func (c Call) Path(dir string) string {
	p := c.str("path")
	if p != "" && !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p
}

// Command is the shell line of a Shell call.
func (c Call) Command() string { return c.str("command") }

// HookInput is the call's input as hooks see it.
func (c Call) HookInput(dir string) map[string]any {
	switch c.Kind {
	case "shellToolCall":
		return map[string]any{"command": c.Command(), "cwd": c.str("workingDirectory"), "timeout": 30000}
	case "readToolCall":
		return map[string]any{"file_path": c.Path(dir)}
	default:
		return map[string]any{"file_path": c.Path(dir), "content": c.str("streamContent")}
	}
}

// Execute runs the call: a shell command in dir with env, or a file tool on
// the file it names. A call of a tool the mock does not model fails.
func Execute(ctx context.Context, c Call, dir string, env []string) Result {
	switch c.Kind {
	case "shellToolCall":
		return shell(ctx, c, dir, env)
	case "readToolCall":
		return read(c, dir)
	case "editToolCall":
		return write(c, dir)
	}
	return Result{Outcome: corehooks.ToolErrored, Frame: map[string]any{"error": map[string]any{"errorMessage": "unsupported tool " + c.Kind}}}
}

func jsonString(v any) string { b, _ := json.Marshal(v); return string(b) }
