package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// flushOwed prints the frames owed.
func (s *session) flushOwed() { s.owed.Release(func(f []byte) { s.forward(f) }) }

// forward prints one stream-json line; a call's started frame first brings out
// what the agent said before it.
func (s *session) forward(line []byte) {
	if bytes.Contains(line, []byte(`"subtype":"started"`)) && bytes.Contains(line, []byte(`"type":"tool_call"`)) {
		s.modelN++ // a new call is a new model response
		s.flushText(true)
	}
	fmt.Fprintf(s.cfg.Stdout, "%s\n", s.stamp(line))
}

// stamp gives an assistant or tool_call frame what every such frame of the
// recordings carries: the clock and the model call it came of, which the run's
// request id, its count and four characters of its own name (recorded: every
// assistant and tool_call frame of runs/*).
func (s *session) stamp(line []byte) []byte {
	var f map[string]any
	if json.Unmarshal(line, &f) != nil || (f["type"] != "assistant" && f["type"] != "tool_call") {
		return line
	}
	f["timestamp_ms"] = time.Now().UnixMilli()
	f["model_call_id"] = s.modelCallID()
	b, err := json.Marshal(f)
	if err != nil {
		return line
	}
	return b
}

func (s *session) modelCallID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s-%d", s.requestID, s.modelN)))
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	suffix := make([]byte, 4)
	for i := range suffix {
		suffix[i] = alphabet[int(sum[i])%len(alphabet)]
	}
	return fmt.Sprintf("%s-%d-%s", s.requestID, s.modelN, suffix)
}

// flushText shows what the agent said so far as one assistant frame. The frame
// brought out by a call carries its model call and the clock; the one at the end
// of the turn carries neither (recorded: runs/file-tools, runs/hook-matchers-mcp).
func (s *session) flushText(atCall bool) {
	if len(s.pending) == 0 {
		return
	}
	text := strings.Join(s.pending, "")
	s.pending = nil
	frame := assistantFrame(s.id, text)
	if atCall {
		frame = s.stamp(frame)
	}
	fmt.Fprintf(s.cfg.Stdout, "%s\n", frame)
}
