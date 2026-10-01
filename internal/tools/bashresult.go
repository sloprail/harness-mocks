package tools

import (
	"fmt"
	"strings"
)

// Message is what the agent is told of a shell command that ran in the
// foreground: its output with the trailing newlines trimmed, and isError
// false; for a command that ended with a non-zero status, an error that
// states the exit code and then the output.
func (r BashResult) Message() (text string, isError bool) {
	out := strings.TrimRight(r.Output, "\n")
	if !r.Failed() {
		return out, false
	}
	msg := fmt.Sprintf("Exit code %d", r.ExitCode)
	if out != "" {
		msg += "\n" + out
	}
	return msg, true
}

// MessageFor is Message for a command some of whose exit status 1 is not a
// failure: when the last command of the pipeline is one of benign (a search or
// a comparison, for which 1 only says nothing matched or the inputs differ),
// the output is a valid result.
func (r BashResult) MessageFor(command string, benign []string) (text string, isError bool) {
	if r.ExitCode == 1 && lastCommandIn(command, benign) {
		return strings.TrimRight(r.Output, "\n"), false
	}
	return r.Message()
}

// lastCommandIn is whether the last command of a shell line (after its last
// "|", "&&", "||" or ";") runs one of names, ignoring leading VAR=value words;
// "git diff" and "git grep" are named as the two words.
func lastCommandIn(command string, names []string) bool {
	seg := command
	for _, sep := range []string{"|", "&&", ";"} {
		if i := strings.LastIndex(seg, sep); i >= 0 {
			seg = seg[i+len(sep):]
		}
	}
	words := strings.Fields(seg)
	for len(words) > 0 && strings.Contains(words[0], "=") && !strings.HasPrefix(words[0], "=") {
		words = words[1:]
	}
	if len(words) == 0 {
		return false
	}
	for _, n := range names {
		parts := strings.Fields(n)
		if len(words) >= len(parts) && strings.Join(words[:len(parts)], " ") == n {
			return true
		}
	}
	return false
}
