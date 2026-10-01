package scenario

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Input is what the script is told on each run, in A10N_MOCK_* variables.
type Input struct {
	// Prompt is the user's prompt, unchanged.
	Prompt string
	// AdditionalContext is the context a prompt hook added, apart from the prompt.
	AdditionalContext string
	// SessionFile is the session's transcript so far.
	SessionFile string
}

// Idents are the variables a script is given.
//
// The script receives the user's prompt, unchanged, in A10N_MOCK_PROMPT; the
// session transcript so far in A10N_MOCK_SESSION_FILE, so it can vary its
// output per turn; and the additional context a prompt hook added in
// A10N_MOCK_ADDITIONAL_CONTEXT, never in place of the prompt.
//
// sr:invariant scenario-prompt-env
// sr:invariant session-file-env
// sr:invariant prompt-context-appended
func Idents(in Input) map[string]string {
	return map[string]string{
		"A10N_MOCK_PROMPT":             in.Prompt,
		"A10N_MOCK_ADDITIONAL_CONTEXT": in.AdditionalContext,
		"A10N_MOCK_SESSION_FILE":       in.SessionFile,
	}
}

// Script is the scenario script that drives a mock.
type Script struct {
	// Path is the script, run with /bin/sh.
	Path string
	// Dir is the working directory it runs in.
	Dir string
	// Environ is the mock's own environment, which the script inherits.
	Environ []string
	// Stderr receives what the script prints on its stderr.
	Stderr io.Writer
}

// Lines runs the script once and returns the non-empty lines it printed on
// stdout, in order. A script that cannot start, or exits non-zero, is an
// error.
func (s Script) Lines(ctx context.Context, in Input) ([][]byte, error) {
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{"/bin/sh", s.Path}, Dir: s.Dir, Stderr: s.Stderr,
		Env: procexec.Env(s.Environ, Idents(in), nil),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot run the scenario script %s: %w", s.Path, err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("scenario script %s exited with status %d", s.Path, res.ExitCode)
	}
	var lines [][]byte
	for _, l := range bytes.Split(res.Stdout, []byte("\n")) {
		if l = bytes.TrimSpace(l); len(l) > 0 {
			lines = append(lines, l)
		}
	}
	return lines, nil
}
