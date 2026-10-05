package runner

import (
	"bytes"
	"fmt"
	"strings"
)

// flushOwed prints the frames owed.
func (s *session) flushOwed() { s.owed.Release(func(f []byte) { s.forward(f) }) }

// forward prints one stream-json line; a call's started frame first brings out
// what the agent said before it.
func (s *session) forward(line []byte) {
	if bytes.Contains(line, []byte(`"subtype":"started"`)) && bytes.Contains(line, []byte(`"type":"tool_call"`)) {
		s.flushText()
	}
	fmt.Fprintf(s.cfg.Stdout, "%s\n", line)
}

// flushText shows what the agent said so far as one assistant frame.
func (s *session) flushText() {
	if len(s.pending) == 0 {
		return
	}
	text := strings.Join(s.pending, "")
	s.pending = nil
	fmt.Fprintf(s.cfg.Stdout, "%s\n", assistantFrame(s.id, text))
}
