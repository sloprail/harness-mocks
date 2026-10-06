package replay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recorded run's options the mock can be given are given; a token limit is dropped (the script
// compacts where the harness did); an option the adapter cannot give is an error, never ignored.
func TestRunFlagsGivesWhatTheMockTakesAndRefusesTheRest(t *testing.T) {
	pass, err := runFlags("-c\nagents.max_depth=3\n-C\n.codex\n--ephemeral\n")
	require.NoError(t, err)
	assert.Equal(t, []string{"-c", "agents.max_depth=3", "-C", ".codex", "--ephemeral"}, pass)

	pass, err = runFlags("-c\nmodel_auto_compact_token_limit=4000\n")
	require.NoError(t, err)
	assert.Empty(t, pass, "the token limit is the script's compactions, not a flag")

	for _, bad := range []string{"--output-schema\nschema.json", "-c\nsomething=1", "resume\nid", "-C"} {
		_, err := runFlags(bad)
		assert.Error(t, err, bad)
	}
}
