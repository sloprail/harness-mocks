package replay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

func fakeMock(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "mock")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755))
	return p
}

// A recording made with a flag the mock refuses replays as the check that the mock refuses it: a non-zero
// exit, a refusal naming the flag, and the script never run.
func TestARefusedFlagRecordingChecksThatTheMockRefusesIt(t *testing.T) {
	rec := core.Recording{Prompt: "p", Setup: map[string]string{"refused-flag": "--output-schema", "refused-args": "--output-schema\nschema.json", "cmdflags": "--json"}}
	run := func(body string) error { return checkRefusal(fakeMock(t, body), rec, os.Environ()) }

	assert.NoError(t, run(`echo "codex-mock: --output-schema is not implemented" >&2; exit 1`))
	assert.Error(t, run(`exit 0`), "the mock ran with the flag")
	assert.Error(t, run(`echo "codex-mock: refused" >&2; exit 1`), "a refusal that does not name the flag")
	assert.Error(t, run(`echo "--output-schema refused" >&2; for a; do case "$prev" in --script) sh "$a";; esac; prev=$a; done; exit 1`), "the script ran")

	flag, words := refusedFlagIn("--output-schema\nschema.json\n")
	assert.Equal(t, "--output-schema", flag)
	assert.Len(t, words, 2)
	flag, _ = refusedFlagIn("-c\nagents.max_depth=3\n")
	assert.Empty(t, flag)
}
