package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The mock implements no feature switch (--enable / --disable): a run asked for
// one is refused, so a recording made with `--enable multi_agent_v2` cannot be
// replayed on the default mode and pass for it.
func TestFeatureSwitchesAreRefused(t *testing.T) {
	for _, flag := range []string{"--enable", "--disable"} {
		t.Run(flag, func(t *testing.T) {
			r := execIn(t, t.TempDir(), "--skip-git-repo-check", flag, "multi_agent_v2", "go")
			assert.NotZero(t, r.Code)
			assert.Contains(t, r.Stderr, "implements no feature switch")
			assert.Empty(t, r.hookLog(), "nothing ran")
		})
	}
}
