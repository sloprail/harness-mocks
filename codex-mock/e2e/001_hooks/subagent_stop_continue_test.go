package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A SubagentStop hook returning continue:false outranks another hook's block:
// the sub-agent is not run again (hooks#subagentstop; the recordings do not
// cover it, so this is the mock's reading of the docs).
// sr:proves subagent-stop-block-loop/codex
func TestSubagentStopContinueFalseOutranksABlock(t *testing.T) {
	got := execMock(t, scenario{
		HooksJSON: `{"hooks":{"SubagentStop":[{"hooks":[{"type":"command","command":"sh block.sh"},{"type":"command","command":"sh stop.sh"}]}]}}`,
		Files: map[string]string{"sub.sh": subScript,
			"block.sh": `cat >/dev/null; echo '{"decision":"block","reason":"AGAIN"}'`,
			"stop.sh":  `cat >/dev/null; echo '{"continue":false}'`},
		Script: spawnThenResult, Prompt: "go",
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	for _, r := range rollouts(t, got.Home) {
		assert.NotContains(t, r, "AGAIN", "the sub-agent was run again")
	}
}
