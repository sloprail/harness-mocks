package replay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/internal/scenario"
)

// A step waits for the sub-agent that ended before it was taken (the first step after its end), and a
// sub-agent's step for as many of the parent's calls as the recording shows started and finished by then.
func TestGatesOrderAgentsByTheRecordedTimes(t *testing.T) {
	t0 := time.Unix(1000, 0)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	sub := Agent{Calls: []Call{{Tool: ToolShell, At: at(3), Done: at(4)}}, FinalAt: at(5)}
	parent := Agent{
		Calls:   []Call{{Tool: ToolSpawn, Sub: &sub, At: at(1), Done: at(2)}, {Tool: ToolShell, At: at(6), Done: at(7)}},
		FinalAt: at(8),
	}
	main := Gates(parent, nil)
	assert.Len(t, main, 3)
	assert.Equal(t, []int{0}, main[1].Ended, "the call taken after the sub-agent's end waits for it")
	assert.Empty(t, main[0].Ended)
	subG := Gates(sub, &parent)
	assert.Equal(t, 1, subG[0].ParentStarted, "the sub-agent's first step: the spawn had started")
	assert.Equal(t, 1, subG[0].ParentDone, "and finished")
}

// A step of an agent whose sub-agent was still running waits for the calls that sub-agent had started by
// then; a step of a sub-agent waits for the agent that started it to have ended when it had; and a call
// that ran a while waits, before it is carried out, for the steps of the agent that started it that came
// while it ran (recorded: claude bgagent, bgagent-nested-launcher).
func TestGatesOrderWhatRanAtTheSameTime(t *testing.T) {
	t0 := time.Unix(1000, 0)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	sub := Agent{Calls: []Call{{Tool: ToolShell, At: at(3), Done: at(9)}}, FinalAt: at(10)}
	parent := Agent{Calls: []Call{{Tool: ToolSpawn, Sub: &sub, At: at(1), Done: at(2)}}, FinalAt: at(5)}
	main := Gates(parent, nil)
	require.Len(t, main, 2)
	assert.Equal(t, []scenario.ChildCalls{{Sub: 0, Calls: 1}}, main[1].ChildStarted, "the answer came after the sub-agent's first call")
	subG := Gates(sub, &parent)
	assert.Equal(t, 2, subG[0].ExecParentSteps, "its call was carried out after the parent's call and answer: they came while it ran")
	ended := Agent{Calls: []Call{{Tool: ToolShell, At: at(7)}}, FinalAt: at(8)}
	assert.True(t, Gates(ended, &parent)[0].ParentEnded, "the agent that started it had ended by its first step")
}

// A call also waits, before it is carried out, for the steps of the agents above its parent that came
// while it ran (recorded: claude bgagent-nested-launcher: the main agent's answer came before the inner
// agent's shell started).
func TestGatesOrderACallAgainstTheAgentsAboveItsParent(t *testing.T) {
	t0 := time.Unix(1000, 0)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	inner := Agent{Calls: []Call{{Tool: ToolShell, At: at(7), Done: at(13)}}, FinalAt: at(14)}
	outer := Agent{Calls: []Call{{Tool: ToolSpawn, Sub: &inner, At: at(4), Done: at(5)}}, FinalAt: at(6)}
	main := Agent{Calls: []Call{{Tool: ToolSpawn, Sub: &outer, At: at(2), Done: at(6)}}, FinalAt: at(9)}
	g := Gates(inner, &outer, &main)
	assert.Equal(t, []int{2}, g[0].ExecAncestorSteps, "the main agent's call and answer came while the inner call ran")
}
