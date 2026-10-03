package subagents

import (
	"context"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanNeutralisesAndReports(t *testing.T) {
	rules := []ScanRule{
		{Name: "flag", Match: regexp.MustCompile(`--danger`), Report: true},
		{Name: "tag", Match: regexp.MustCompile(`<(/?tag)`), Rewrite: `<\$1`, Report: true},
		{Name: "role", Match: regexp.MustCompile(`(?m)^(Human):`), Rewrite: `$1\:`},
	}
	got, matched := Scan("a <tag> and --danger\nHuman: hi", rules)
	assert.Equal(t, "a <\\tag> and --danger\nHuman\\: hi", got)
	assert.Equal(t, []string{"flag", "tag"}, matched, "an unreported rewrite is not listed")
	got, matched = Scan("plain", rules)
	assert.Equal(t, "plain", got)
	assert.Empty(t, matched)
}

func TestTurnLimitStopsAtMax(t *testing.T) {
	var none *TurnLimit
	assert.False(t, none.Step())
	assert.False(t, none.Reached())
	l := &TurnLimit{Max: 2}
	assert.False(t, l.Step())
	assert.False(t, l.Reached())
	assert.True(t, l.Step())
	assert.True(t, l.Reached())
	assert.Same(t, l, LimitOf(WithLimit(context.Background(), l)))
	assert.Nil(t, LimitOf(WithLimit(WithLimit(context.Background(), l), nil)), "nil lifts an enclosing limit")
}

func TestLimitedSkipsTheStopHookOnceAtTheLimit(t *testing.T) {
	calls := 0
	h := Hooks{Stop: func(bool, string) (bool, string) { calls++; return true, "again" }}
	l := &TurnLimit{Max: 1}
	limited := Limited(h, l)
	blocked, _ := limited.Stop(false, "x")
	assert.True(t, blocked)
	l.Step()
	blocked, _ = limited.Stop(false, "x")
	assert.False(t, blocked)
	assert.Equal(t, 1, calls)
}

func TestTallyCountsByClass(t *testing.T) {
	classes := map[string]Class{"Read": Read, "Bash": Shell, "Edit": Edit, "Grep": Search, "Agent": Skipped}
	c := Tally([]Call{
		{Tool: "Read"}, {Tool: "Bash"}, {Tool: "Bash"}, {Tool: "Grep"}, {Tool: "Agent"}, {Tool: "ToolSearch"},
		{Tool: "Edit", Added: "a\nb", Taken: "c"},
	}, classes)
	assert.Equal(t, Counts{Read: 1, Search: 1, Shell: 2, Edits: 1, Other: 1, LinesAdded: 2, LinesRemoved: 1}, c)
	assert.Equal(t, 6, c.Total())
}

func TestConcurrentLimit(t *testing.T) {
	assert.Equal(t, 20, ConcurrentLimit(""))
	assert.Equal(t, 3, ConcurrentLimit("3"))
	for _, bad := range []string{"0", "-2", "1.5", "two", "1e3", " 4"} {
		assert.Equal(t, DefaultConcurrentLimit, ConcurrentLimit(bad), bad)
	}
	assert.False(t, AtConcurrentLimit(19, 20))
	assert.True(t, AtConcurrentLimit(20, 20))
}
