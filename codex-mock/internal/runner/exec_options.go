package runner

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/toolcall"
)

// execOptions are the options of Codex's exec_command the mock looks at.
type execOptions struct {
	TTY             bool    `json:"tty"`
	Workdir         *string `json:"workdir"`
	MaxOutputTokens *int    `json:"max_output_tokens"`
	Shell           *string `json:"shell"`
	Login           *bool   `json:"login"`
}

// unimplemented is what an exec_command asks for that the mock does not carry
// out, refused instead of ignored (adr/fail-fast-unimplemented): a working
// directory other than the run's, a shell other than zsh (the one the
// recordings name), zsh when it is not installed, and a login without a shell.
//
// shellArgv is what runs the shell: the command by the shell the call names,
// zsh -c, or zsh -lc for a login shell. A tty is carried out as far as the
// recordings show it: the terminal's line ending (ttyOutput).
// max_output_tokens only caps what the model is shown: the mock has no model, so
// it matters only once a command's output would exceed it (tooLong).
func (h toolHost) unimplemented(c toolcall.Call) string {
	var o execOptions
	_ = json.Unmarshal(c.Input, &o)
	if o.Workdir != nil && !sameDir(*o.Workdir, h.cfg.Cwd) {
		return "workdir other than the run's directory"
	}
	switch {
	case o.Shell != nil && *o.Shell != "zsh":
		return "shell " + *o.Shell + " (only zsh, the recorded one, is accepted)"
	case o.Shell == nil && o.Login != nil:
		return "login without a shell"
	case o.Shell != nil && zsh() == "":
		return "shell zsh: zsh is not installed on this machine (it is not replaced by /bin/sh)"
	}
	return ""
}

// zsh is the path of the zsh on this machine, empty when there is none.
var zsh = func() string {
	p, _ := exec.LookPath("zsh")
	return p
}

// shellArgv is the command line a command is run by: the shell the call names,
// `zsh -c`, or `zsh -lc` for a login shell, as the harness runs it; /bin/sh -c
// when the call names none. A named shell that is not installed is refused
// before this (unimplemented), never replaced.
func shellArgv(c toolcall.Call, cmd string) []string {
	var o execOptions
	_ = json.Unmarshal(c.Input, &o)
	if o.Shell == nil {
		return []string{"/bin/sh", "-c", cmd}
	}
	flag := "-c"
	if o.Login != nil && *o.Login {
		flag = "-lc"
	}
	return []string{zsh(), flag, cmd}
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
