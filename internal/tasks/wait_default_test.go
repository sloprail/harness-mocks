package tasks

import (
	"testing"
	"time"
)

// A non-interactive run waits ten minutes idle by default for its background
// agents (headless#background-tasks-at-exit).
func TestTheDefaultIdleCeilingIsTenMinutes(t *testing.T) {
	if DefaultWaitCeiling != 10*time.Minute {
		t.Fatalf("DefaultWaitCeiling = %v", DefaultWaitCeiling)
	}
}
