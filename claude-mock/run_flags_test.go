package main

import (
	"testing"
	"time"
)

// The wait for background agents after a `claude -p` run's last turn ends at ten minutes unless
// CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS says otherwise (0 waits without one).
// sr:proves print-waits-for-background-agents/claude
func TestPrintWaitCeilingDefaultsToTenMinutes(t *testing.T) {
	t.Setenv("CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS", "")
	if got := printWaitCeiling(); got != 10*time.Minute {
		t.Fatalf("default: %v", got)
	}
	t.Setenv("CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS", "400")
	if got := printWaitCeiling(); got != 400*time.Millisecond {
		t.Fatalf("set: %v", got)
	}
	t.Setenv("CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS", "0")
	if got := printWaitCeiling(); got != 0 {
		t.Fatalf("zero: %v", got)
	}
}
