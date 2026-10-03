package runner

import (
	"strconv"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// definitionTurnLimit is the limit its definition sets (`maxTurns` in the
// frontmatter of the sub-agent's file), nil when it sets none. A sub-agent at
// it stops with no report and no SubagentStop (recorded:
// snapshots/runs/fgsub-maxturns: maxTurns 2 ran two commands, and the Agent call
// returned at once with a note).
//
// sr:provides foreground-subagent-result/claude
func definitionTurnLimit(cfg Config, name string) *subagents.TurnLimit {
	n, err := strconv.Atoi(definitionField(cfg, name, "maxTurns"))
	if err != nil || n <= 0 {
		return nil
	}
	return &subagents.TurnLimit{Max: n}
}
