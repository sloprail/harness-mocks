package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The frames of Cursor's stream-json output that the mock itself writes
// (recorded: runs/*/samples/*/stream.jsonl). The tool frames are in
// toolcall_frames.go.

// modelNames are the names the init frame gives the models a run was started
// with (--model), as recorded: the default, and the model whose thinking the
// hooks report (runs/hook-matchers-thought).
var modelNames = map[string]string{"": "Auto", "auto": "Auto", "cursor-grok-4.5-high": "Grok 4.5 High"}

func initFrame(session, dir, model string) []byte {
	return jsonLine(map[string]any{
		"type": "system", "subtype": "init", "apiKeySource": "login", "cwd": dir,
		"session_id": session, "model": modelNames[model], "permissionMode": "default",
	})
}

func userFrame(session, prompt string) []byte {
	return jsonLine(map[string]any{
		"type": "user", "session_id": session,
		"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": prompt}}},
	})
}

func assistantFrame(session, text string) []byte {
	return jsonLine(map[string]any{
		"type": "assistant", "session_id": session,
		"message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}}},
	})
}

// resultFrame ends the stream: the run succeeded, with what the agent said.
func resultFrame(session, request, text string, took time.Duration) []byte {
	return jsonLine(map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "result": text, "session_id": session,
		"duration_ms": took.Milliseconds(), "duration_api_ms": took.Milliseconds(), "request_id": request,
		"usage": map[string]any{"inputTokens": 0, "outputTokens": 0, "cacheReadTokens": 0, "cacheWriteTokens": 0},
	})
}

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
	now := time.Now().UnixMilli()
	f["timestamp_ms"] = now
	if tc, ok := f["tool_call"].(map[string]any); ok { // a call's frames say when it began and, once it has ended, when (recorded: every tool_call frame)
		tc["startedAtMs"] = strconv.FormatInt(now, 10)
		if f["subtype"] == "completed" {
			tc["completedAtMs"] = strconv.FormatInt(now, 10)
		}
	}
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
