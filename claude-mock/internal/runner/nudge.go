package runner

import "encoding/json"

// noVisibleOutput is what the harness tells a model whose response had nothing to show.
const noVisibleOutput = "[Your previous response had no visible output. Please continue and produce a user-visible response.]"

// thinkingOnly is whether an assistant line holds thinking and nothing else.
func thinkingOnly(line []byte) bool {
	var rec struct {
		Message *struct {
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &rec) != nil || rec.Message == nil || len(rec.Message.Content) == 0 {
		return false
	}
	for _, b := range rec.Message.Content {
		if b.Type != "thinking" {
			return false
		}
	}
	return true
}

// writeNoVisibleOutputNudge answers a response with no visible output: a meta user record in the
// transcript, a synthetic user frame in the stream, and the response counts as a turn of the run.
func writeNoVisibleOutputNudge(cfg Config, bg *backgroundTasks, tr *transcript) {
	tr.persistMap(map[string]any{
		"type": "user", "isMeta": true, "turnCompanion": true,
		"message": map[string]any{"role": "user", "content": noVisibleOutput},
	})
	if line, err := marshalRecord(map[string]any{
		"type": "user", "isSynthetic": true,
		"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": noVisibleOutput}}},
	}); err == nil {
		writeStreamLine(cfg, line)
	}
	bg.run.turn()
}
