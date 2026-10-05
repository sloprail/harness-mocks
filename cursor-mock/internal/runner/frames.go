package runner

import (
	"time"
)

// The frames of Cursor's stream-json output that the mock itself writes
// (recorded: runs/*/samples/*/stream.jsonl). The tool frames are in
// toolcall_frames.go.

func initFrame(session, dir string) []byte {
	return jsonLine(map[string]any{
		"type": "system", "subtype": "init", "apiKeySource": "login", "cwd": dir,
		"session_id": session, "model": "Auto", "permissionMode": "default",
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
