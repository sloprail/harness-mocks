package runner

import "github.com/sloprail/harness-mocks/claude-mock/internal/toolexec"

// writeMetaMessages writes the messages the harness put before the agent after a tool's result (a launched
// skill's instructions): a meta user record each in the transcript, a synthetic user frame in the stream,
// and a turn of the run (recorded: snapshots/runs/skill-tool, num_turns 7 for four responses and three messages).
func writeMetaMessages(cfg Config, bg *backgroundTasks, call pendingToolUse, res toolexec.Result, tr *transcript) {
	for _, m := range res.Meta {
		block := []any{map[string]any{"type": "text", "text": m.Text}}
		var stored any = block
		if m.Plain {
			stored = m.Text
		}
		tr.persistMap(map[string]any{
			"type": "user", "isMeta": true, "turnCompanion": true, "sourceToolUseID": call.ToolUseID,
			"message": map[string]any{"role": "user", "content": stored},
		})
		if line, err := marshalRecord(map[string]any{
			"type": "user", "isSynthetic": true, "uuid": newRecordUUID(),
			"message": map[string]any{"role": "user", "content": block},
		}); err == nil {
			writeStreamLine(cfg, stampFrame(cfg, line))
		}
		bg.run.turn() // each is a turn of the run (the result's num_turns counts it)
	}
}
