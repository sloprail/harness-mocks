package hooks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Hooks run in the background run one after another in the order started, and what they printed is
// due once the agent has gone through the call after the one it had begun when they started, or when
// its turn ends; delivery waits for them to have finished, and nothing waits for a time.
func TestBackgroundHooksAreDeliveredAtTheNextSafePoint(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	rt := Runtime{Dir: dir}
	cmd := func(name string) []Command {
		return []Command{{Line: "echo " + name + " >>" + log + "; echo out-" + name}}
	}
	var l Later
	l.Start(context.Background(), "A", 0, cmd("a"), nil, rt)
	l.Start(context.Background(), "B", 1, cmd("b"), nil, rt)
	l.Start(context.Background(), "C", 0, cmd("c"), nil, rt)

	events := func(ds []Delivered) (out []string) {
		for _, d := range ds {
			out = append(out, d.Event+":"+strings.TrimSpace(d.Outcomes[0].Stdout))
		}
		return out
	}
	assert.Empty(t, l.Due(0, false), "nothing is due before the agent has gone through a call")
	assert.Equal(t, []string{"A:out-a", "C:out-c"}, events(l.Due(1, false)), "those started before the first call, in the order started")
	assert.Equal(t, []string{"B:out-b"}, events(l.Due(1, true)), "the rest when the turn ends")
	assert.Empty(t, l.Due(5, true))
	l.Wait()
	b, _ := os.ReadFile(log)
	assert.Equal(t, "a\nb\nc\n", string(b), "run one after another, in the order started")
}

// The context a hook the agent waited for adds is due at once; one a background hook adds is not,
// until the agent has gone through a later call, or its turn ends.
func TestContextDue(t *testing.T) {
	assert.True(t, ContextDue(false, 0, 0, false), "waited for: at once")
	assert.False(t, ContextDue(true, 0, 0, false), "background: not yet")
	assert.False(t, ContextDue(true, 1, 1, false), "started during the call just taken")
	assert.True(t, ContextDue(true, 0, 1, false), "started before the call just taken")
	assert.True(t, ContextDue(true, 3, 3, true), "the turn ends")
}
