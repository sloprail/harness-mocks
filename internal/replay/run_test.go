package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type stubAdapter struct{ want, got Observed }

func (stubAdapter) Load(string) (Recording, error)                         { return Recording{}, nil }
func (stubAdapter) Script(Recording) (string, error)                       { return "", nil }
func (a stubAdapter) Replay(string, Recording) (Observed, Observed, error) { return a.want, a.got, nil }

// Two empty event streams compared nothing: that is not a green replay.
func TestNothingComparedIsNotGreen(t *testing.T) {
	diff, err := Run(stubAdapter{}, "", "")
	assert.NoError(t, err)
	assert.Contains(t, diff, "nothing was compared")
	diff, _ = Run(stubAdapter{want: Observed{Events: []string{"a"}}, got: Observed{Events: []string{"a"}}}, "", "")
	assert.Empty(t, diff)
}

// A replay that was a check the adapter made itself (a refused flag) and passed is green with nothing compared;
// one side alone saying so is not.
func TestACheckThatPassedIsGreenWithNothingCompared(t *testing.T) {
	diff, _ := Run(stubAdapter{want: Observed{Checked: true}, got: Observed{Checked: true}}, "", "")
	assert.Empty(t, diff)
	diff, _ = Run(stubAdapter{want: Observed{Checked: true}}, "", "")
	assert.Contains(t, diff, "nothing was compared")
}
