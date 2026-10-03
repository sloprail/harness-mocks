package events

import (
	"fmt"
	"io"
	"strings"
)

// Header is what the progress banner names of the run: the release, the
// working directory, the model, and the prompt the user gave.
type Header struct {
	Version, Cwd, Model, Prompt string
}

// progress is the account `codex exec` gives of a run on stderr when it prints
// no JSONL (recorded: runs/noninteractive-run-text-output): a banner naming the
// release, directory, model and session, the prompt, a warning per notice, each
// command with how it ended and what it printed, each message of the agent, and
// the tokens used.
type progress struct {
	w io.Writer
	h Header
}

// Progress makes the stream also write that account to w. What the banner
// says of the configuration the mock has none of (provider, approval, sandbox,
// reasoning), a line per hook run and the token counts are not modeled.
func (s *Stream) Progress(w io.Writer, h Header) { s.text = &progress{w: w, h: h} }

// Warning reports a notice of the harness about the run: an item of type error
// in the event stream, a line `warning: <message>` in the progress (recorded:
// the --dangerously-bypass-hook-trust notice, and the one for an async SessionEnd hook).
func (s *Stream) Warning(message string) {
	s.emit(map[string]any{"type": "item.completed", "item": map[string]any{
		"id": s.newID(), "type": "error", "message": message}})
}

func (p *progress) render(v map[string]any) {
	item, _ := v["item"].(map[string]any)
	switch v["type"] {
	case "thread.started":
		fmt.Fprintf(p.w, "OpenAI Codex v%s\n--------\nworkdir: %s\nmodel: %s\nsession id: %s\n--------\nuser\n%s\n",
			p.h.Version, p.h.Cwd, p.h.Model, v["thread_id"], p.h.Prompt)
	case "item.started":
		if item["type"] == "command_execution" {
			fmt.Fprintf(p.w, "exec\n%s in %s\n", item["command"], p.h.Cwd)
		}
	case "item.completed":
		switch item["type"] {
		case "error":
			fmt.Fprintf(p.w, "warning: %s\n", item["message"])
		case "agent_message":
			fmt.Fprintf(p.w, "codex\n%s\n", item["text"])
		case "command_execution":
			how := "succeeded"
			if code, _ := item["exit_code"].(int); code != 0 {
				how = fmt.Sprintf("exited %d", code)
			}
			out, _ := item["aggregated_output"].(string)
			if out != "" && !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			fmt.Fprintf(p.w, " %s in 0ms:\n%s\n", how, out)
		}
	case "turn.completed":
		fmt.Fprint(p.w, "tokens used\n0\n")
	}
}
