package runner

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// execOptions are the options of Codex's exec_command the mock looks at.
type execOptions struct {
	TTY             bool    `json:"tty"`
	Workdir         *string `json:"workdir"`
	MaxOutputTokens *int    `json:"max_output_tokens"`
}

// unimplemented is what an exec_command asks for that the mock does not carry
// out, refused instead of ignored (adr/fail-fast-unimplemented): a working
// directory other than the run's. A tty is carried out as far as the recordings
// show it: the terminal's line ending (ttyOutput). shell and login only choose
// the shell, and every command of a scenario is run by /bin/sh. max_output_tokens only caps what the model
// is shown: the mock has no model, so it matters only once a command's output
// would exceed it (tooLong).
func (h toolHost) unimplemented(c toolcall.Call) string {
	var o execOptions
	_ = json.Unmarshal(c.Input, &o)
	if o.Workdir != nil && !sameDir(*o.Workdir, h.cfg.Cwd) {
		return "workdir other than the run's directory"
	}
	return ""
}

// tooLong reports whether a command's output is longer than max_output_tokens
// allows (a token is about four bytes): Codex then truncates what it tells the
// model, which the mock does not do.
func tooLong(c toolcall.Call, output string) bool {
	var o execOptions
	_ = json.Unmarshal(c.Input, &o)
	return o.MaxOutputTokens != nil && len(output) > 4**o.MaxOutputTokens
}

// ttyOutput is a command's output as a pseudo-terminal gives it: lines end in
// carriage return and newline (recorded: runs/task-notifications-bg, "BGDONE\r\n").
func ttyOutput(c toolcall.Call, output string) string {
	var o execOptions
	_ = json.Unmarshal(c.Input, &o)
	if !o.TTY {
		return output
	}
	return strings.ReplaceAll(strings.ReplaceAll(output, "\r\n", "\n"), "\n", "\r\n")
}

func sameDir(a, b string) bool {
	ra, ea := filepath.EvalSymlinks(a)
	rb, eb := filepath.EvalSymlinks(b)
	if ea != nil || eb != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

func refused(option string) toolcall.Result {
	return toolcall.Result{Failed: true, Output: fmt.Sprintf("codex-mock: exec_command %s is not implemented by the mock: it is refused rather than ignored", option)}
}
