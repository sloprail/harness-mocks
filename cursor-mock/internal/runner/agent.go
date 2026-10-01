package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sloprail/harness-mocks/cursor-mock/internal/toolexec"
	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/turnloop"
)

// frame is the part of a stream-json line the mock reads.
type frame struct {
	Type     string                     `json:"type"`
	Subtype  string                     `json:"subtype"`
	CallID   string                     `json:"call_id"`
	ToolCall map[string]json.RawMessage `json:"tool_call"`
	Message  struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// Turn runs the scenario script once and plays its lines: each is printed as
// it comes, up to the first tool_call frame that starts a call, which ends the
// turn, or the result frame, which ends the run (it is held, and printed last).
func (s *session) Turn(ctx context.Context) (turnloop.Step[pending], error) {
	lines, err := scenario.Script{Path: s.cfg.Script, Dir: s.cfg.Dir, Environ: s.cfg.Environ, Stderr: s.cfg.Stderr}.
		Lines(ctx, scenario.Input{Prompt: s.cfg.Prompt, SessionFile: s.tr.path})
	if err != nil {
		return turnloop.Step[pending]{}, err
	}
	for _, line := range lines {
		var f frame
		if err := json.Unmarshal(line, &f); err != nil || f.Type == "" {
			return turnloop.Step[pending]{}, fmt.Errorf("the scenario script printed an invalid stream-json line: %s", line)
		}
		switch {
		case f.Type == "result":
			s.result = line
			return turnloop.Step[pending]{Done: true}, nil
		case f.Type == "tool_call" && f.Subtype == "started":
			s.forward(line)
			p, err := startedCall(f)
			if err != nil {
				return turnloop.Step[pending]{}, err
			}
			s.tr.toolUse(p.call.Name(), p.call.Args)
			return turnloop.Step[pending]{Call: p, Key: p.call.Kind + string(jsonLine(p.call.Args))}, nil
		default:
			s.forward(line)
			if f.Type == "assistant" && len(f.Message.Content) > 0 {
				s.tr.text(f.Message.Content[0].Text)
			}
		}
	}
	return turnloop.Step[pending]{Done: true}, nil
}

// startedCall is the call a tool_call started frame names.
func startedCall(f frame) (pending, error) {
	for kind, raw := range f.ToolCall {
		c := toolexec.Call{Kind: kind}
		if c.Name() == "" {
			continue
		}
		var body struct {
			Args map[string]any `json:"args"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return pending{}, fmt.Errorf("a tool_call frame's %s is not an object: %w", kind, err)
		}
		c.Args = body.Args
		return pending{call: c, id: f.CallID}, nil
	}
	return pending{}, fmt.Errorf("a tool_call frame names no tool the mock runs (shellToolCall, readToolCall, editToolCall)")
}
