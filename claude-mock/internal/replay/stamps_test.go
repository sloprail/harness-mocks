package replay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rec(kind, stamp string, content ...any) map[string]any {
	return map[string]any{"type": kind, "timestamp": stamp, "message": map[string]any{"content": content}}
}

func use(id string) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": "true"}}
}

func result(id string) map[string]any {
	return map[string]any{"type": "tool_result", "tool_use_id": id, "content": "ok"}
}

func text(s string) map[string]any { return map[string]any{"type": "text", "text": s} }

// A call carries when the model made it and when its result came back, and the final answer when it was
// given: what the gates order the agents' steps by.
func TestModelTurnsRecordWhenEachStepHappened(t *testing.T) {
	got, err := modelTurns([]map[string]any{
		rec("assistant", "2026-01-01T00:00:01.000Z", use("a")),
		rec("user", "2026-01-01T00:00:02.000Z", result("a")),
		rec("assistant", "2026-01-01T00:00:03.000Z", text("done")),
	})
	require.NoError(t, err)
	at := func(s string) time.Time { v, _ := time.Parse(time.RFC3339Nano, s); return v }
	assert.Equal(t, at("2026-01-01T00:00:01Z"), got.agent.Calls[0].At)
	assert.Equal(t, at("2026-01-01T00:00:02Z"), got.agent.Calls[0].Done)
	assert.Equal(t, at("2026-01-01T00:00:03Z"), got.agent.FinalAt)
}

// A fork's transcript opens with the context it inherited: the call that started it and the result that
// answered that call. They are not what the fork did.
func TestAForkDoesNotReplayTheContextItInherited(t *testing.T) {
	got, err := modelTurns([]map[string]any{
		{"type": "fork-context-ref"},
		rec("assistant", "2026-01-01T00:00:01Z", use("parent")),
		rec("user", "2026-01-01T00:00:02Z", result("parent")),
		rec("assistant", "2026-01-01T00:00:03Z", use("own")),
		rec("user", "2026-01-01T00:00:04Z", result("own")),
	})
	require.NoError(t, err)
	require.Len(t, got.agent.Calls, 1)
	assert.Equal(t, "own", got.ids[0])
}
