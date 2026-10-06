package replay

import (
	"fmt"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// attachSubs gives each spawn_agent call of the agent that was answered with a receipt the turns of the
// sub-agent it started, read from that thread's rollout (taken out of subs), and does the same for the
// sub-agents that one started in turn: its rollout holds the receipts it was given.
func attachSubs(a *core.Agent, subs map[string][]map[string]any) error {
	for i := range a.Calls {
		c := &a.Calls[i]
		if c.Tool != core.ToolSpawn || c.Ref == "" { // a spawn with no receipt was refused: no sub-agent
			continue
		}
		rollout, ok := subs[c.Ref]
		if !ok {
			return fmt.Errorf("a spawn_agent whose receipt names %s, which has no recorded rollout", c.Ref)
		}
		delete(subs, c.Ref)
		sub, err := modelTurns(rollout, nil)
		if err != nil {
			return fmt.Errorf("sub-agent: %w", err)
		}
		if err := attachSubs(&sub, subs); err != nil {
			return err
		}
		c.Sub = &sub
	}
	return nil
}
