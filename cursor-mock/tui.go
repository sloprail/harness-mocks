package main

import (
	"errors"
	"io"
	"strings"
)

// typedPrompt is what the user types in a TUI session (cursor-agent started without -p), which
// here is stdin: one prompt, then the slash commands typed at the idle input after its turn. Only
// /compress was recorded (runs/tui-manual-compaction); a prompt argument (the TUI's initial
// prompt), a second prompt and any other command were not, so they are refused.
func typedPrompt(args []string, stdin io.Reader) (string, []string, error) {
	if len(args) > 0 {
		return "", nil, errors.New("cursor-mock: a prompt argument without -p is not modeled: the prompt is read from stdin")
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return "", nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if lines[0] == "" {
		return "", nil, errors.New("cursor-mock: a TUI session is modeled with a prompt line on stdin")
	}
	for _, l := range lines[1:] {
		if l != "/compress" {
			return "", nil, errors.New("cursor-mock: a TUI session is modeled with one prompt line on stdin, then only /compress lines: " + l)
		}
	}
	return lines[0], lines[1:], nil
}
