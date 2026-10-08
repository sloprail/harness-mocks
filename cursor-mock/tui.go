package main

import (
	"errors"
	"io"
	"strings"
)

// typedInputs is what the user types in a TUI session (cursor-agent started without -p), which
// here is stdin, one line each: prompts, each a turn of the one conversation (runs/tui-multi-turn),
// and /compress typed at the idle input after a turn (runs/tui-manual-compaction). A prompt
// argument (the TUI's initial prompt), an empty line and any other slash command were not
// recorded, so they are refused.
func typedInputs(args []string, stdin io.Reader) ([]string, error) {
	if len(args) > 0 {
		return nil, errors.New("cursor-mock: a prompt argument without -p is not modeled: the prompts are read from stdin")
	}
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if lines[0] == "" {
		return nil, errors.New("cursor-mock: a TUI session is modeled with a prompt line on stdin")
	}
	for i, l := range lines {
		switch {
		case l == "":
			return nil, errors.New("cursor-mock: an empty line typed in a TUI session is not modeled")
		case strings.HasPrefix(l, "/") && l != "/compress":
			return nil, errors.New("cursor-mock: a TUI session is modeled with prompts on stdin, one per line, and /compress lines after the first prompt: " + l)
		case i == 0 && l == "/compress":
			return nil, errors.New("cursor-mock: /compress typed before any prompt is not modeled")
		}
	}
	return lines, nil
}
