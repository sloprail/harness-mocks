package hooks

import "testing"

func TestAfterRefusedTool(t *testing.T) {
	if got := AfterRefusedTool(false); got != AfterNone {
		t.Errorf("AfterRefusedTool(false) = %d, want AfterNone", got)
	}
	if got := AfterRefusedTool(true); got != AfterFailure {
		t.Errorf("AfterRefusedTool(true) = %d, want AfterFailure", got)
	}
}
