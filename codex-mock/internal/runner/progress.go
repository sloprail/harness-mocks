package runner

import (
	"context"
	"fmt"

	"github.com/sloprail/harness-mocks/internal/scenario"
	"github.com/sloprail/harness-mocks/internal/subagents"
)

// Gate holds the agent's step back for what its script's gate names: the core decides
// what that is and when it has happened (subagents.Hold); the mock reports a gate that
// names what does not exist, in its own words.
func (h *state) Gate(ctx context.Context, g scenario.Gate) {
	for _, p := range subagents.Hold(ctx, g, h.bg, h.spawned, h.parent) {
		fmt.Fprintf(h.cfg.Stderr, "ERROR codex_mock: %s\n", p)
	}
}
