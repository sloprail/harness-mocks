package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

func TestScratchTextTakesOutWhatIsTheMachines(t *testing.T) {
	assert.Equal(t, "Preparing worktree\nHEAD is now at <SHA> init\n/x\n",
		scratchText("Preparing worktree\nHEAD is now at 84f430e init\n/x\n"))
	assert.Equal(t, "A=1\nCODEX_MANAGED_PACKAGE_ROOT=<PKG_ROOT>\nB=2\n",
		scratchText("A=1\nCODEX_MANAGED_PACKAGE_ROOT=/home/me/.nvm/lib/codex\nB=2\n"))
	assert.Equal(t, "plain text", scratchText("plain text"))
}

// A sub-agent's script is not in the repository, as the real run's has none.
func TestSubAgentScriptsSitBesideTheRepository(t *testing.T) {
	sub := &core.Agent{Final: "ok"}
	rec := core.Recording{Agent: core.Agent{Calls: []core.Call{{Tool: core.ToolSpawn, Input: map[string]any{"message": "m"}, Sub: sub}}}}
	s := Denormalize(rec)
	assert.Contains(t, s.Scripts, "sub0.sh")
	assert.NotContains(t, s.Files, "sub0.sh")
	assert.Contains(t, s.Script, scriptsDir+"/sub0.sh")
}
