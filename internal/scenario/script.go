// Package scenario runs the script that drives a mock: once per turn, it
// prints stream-json lines that say what the agent does.
package scenario

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/procexec"
)

// Input is what the script is told on each run, in A10N_MOCK_* variables.
type Input struct {
	// Prompt is the user's prompt, unchanged.
	Prompt string
	// AdditionalContext is the context a UserPromptSubmit hook added.
	AdditionalContext string
	// SessionFile is the session record so far.
	SessionFile string
}

// ToolUse is a tool call the script asked for.
type ToolUse struct {
	ID    string
	Name  string
	Input json.RawMessage
	// More is that another call of the same script of the model follows this one: the
	// model is not asked again between them, so nothing is told the agent in between.
	More bool
}

// Compact is a request of the script to compact the session, naming what
// triggered it ("manual" or "auto"; empty when it names none).
type Compact struct {
	Trigger string
	// Fields are the other keys of the compact line: what the harness says of the
	// compaction (token counts, say), which the core leaves to the harness.
	Fields map[string]json.RawMessage
}

// Thought is a thinking block of a script: the text, and the other keys the block
// holds (the model that thought, say), which the core leaves to the harness.
type Thought struct {
	Text   string
	Fields map[string]json.RawMessage
}

// Turn is what one run of the script said: the agent's messages in order, and
// how the turn ends, with tool calls or with the result that ends the run
// (neither: the script ended without a tool call or a result).
type Turn struct {
	Texts []string
	// Thoughts are what the model thought before it answered or called, in order:
	// the script's thinking blocks, which a mock that has no model otherwise
	// never says.
	Thoughts []Thought
	// Tool is the turn's first call and Tools all of them, in order: a turn has
	// several when the script prints tool_use lines in a row.
	Tool  *ToolUse
	Tools []ToolUse
	// Compact is the compaction the turn ends with, when it asked for one.
	Compact *Compact
	Result  *string
	// Gate is what the script says must have happened before the turn's calls and
	// messages are taken: the order of the agents' steps, set by the script and not by
	// how long anything takes.
	Gate Gate
}

// Idents are the variables a script is given.
//
// The script receives the user's prompt, unchanged, in A10N_MOCK_PROMPT; the
// session record so far in A10N_MOCK_SESSION_FILE; and a UserPromptSubmit
// hook's additional context in A10N_MOCK_ADDITIONAL_CONTEXT, never in place of
// the prompt.
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

// RunTurn runs the script once and reads its lines up to the first tool call
// (with the tool_use lines that follow it) or result.
//
// The script runs once per turn and prints stream-json lines: tool_use lines
// end the turn, the mock runs the tools and runs the script again; a result
// line ends the run.
//
// sr:invariant turn-loop
func RunTurn(ctx context.Context, script, dir string, environ []string, in Input) (Turn, error) {
	res, err := procexec.Run(ctx, procexec.Spec{
		Argv: []string{"/bin/sh", script}, Dir: dir, Env: procexec.Env(environ, Idents(in), nil),
	})
	if err != nil {
		return Turn{}, fmt.Errorf("codex-mock: cannot run the scenario script: %w", err)
	}
	var t Turn
	sc := bufio.NewScanner(bytes.NewReader(res.Stdout))
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		done, err := t.read(line)
		if err != nil {
			return Turn{}, fmt.Errorf("codex-mock: scenario script printed an invalid line: %w\nline: %s", err, line)
		}
		if done {
			break
		}
	}
	return t, nil
}
