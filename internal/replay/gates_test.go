package replay

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
