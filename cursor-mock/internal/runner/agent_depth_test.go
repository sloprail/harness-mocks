package runner

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// A Task call of a sub-agent at the depth limit is answered as a tool the agent
// does not have: an "Unknown tool: Task" error frame on the stream and the call
// in the transcript, where the recorded agent said it had no Task tool
// (runs/nested-subagents-depth); a sub-agent one layer above the limit is not
// refused.
// sr:proves nested-subagents/cursor
func TestATaskCallAtTheDepthLimitIsAnUnknownToolAnswer(t *testing.T) {
	main := &session{id: "main"}
	first := &session{id: "first", parent: main}
	second := &session{id: "second", parent: first}
	require.Equal(t, agentDepthLimit, second.depth())

	call := scenario.ToolUse{ID: "t1", Name: "Task", Input: []byte(`{"prompt":"go","description":"d","subagent_type":"generalPurpose"}`)}
	for _, c := range []struct {
		s       *session
		refused bool
	}{{first, false}, {second, true}} {
		var out bytes.Buffer
		home := t.TempDir()
		tr, err := newTranscript(home, "/ws", c.s.id)
		require.NoError(t, err)
		c.s.tr, c.s.cfg.Stdout = tr, &out
		c.s.refusal = &refusal{cancel: func() {}}

		assert.Equal(t, c.refused, c.s.refusesTaskAtTheLimit(context.Background(), call), c.s.id)
		if !c.refused {
			assert.Empty(t, out.String())
			continue
		}
		assert.Contains(t, out.String(), `"errorMessage":"Unknown tool: Task"`)
		assert.Contains(t, out.String(), `"completed"`)
		b, err := os.ReadFile(tr.path)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(string(b), `"name":"Task"`), "the call is left in the transcript")
	}
}
