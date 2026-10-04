package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	codexreplay "github.com/sloprail/harness-mocks/codex-mock/internal/replay"
)

// Every recorded run, replayed by the mock's own replay (codexreplay.Run, which
// `a10n-codex-mock replay <run-dir>` runs too): the mock is given the run's own
// setup and the model's own turns, as a scenario script it generates in the
// format any scenario has, and its whole event stream and hook payloads are
// compared with what the real codex left. The table is the recording folders
// themselves: a recording without a replay, or a replay without a recording,
// cannot exist. A run that does not replay green is listed in notReplaying
// with its reason (replay_allowlist_test.go).
func TestGeneratedReplay(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join(runsDir, "*"))
	require.NoError(t, err)
	var names []string
	for _, d := range dirs {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			names = append(names, filepath.Base(d))
		}
	}
	sort.Strings(names)
	for name := range notReplaying {
		if _, err := os.Stat(filepath.Join(runsDir, name)); err != nil {
			t.Errorf("notReplaying lists %s, which has no recording: remove the entry", name)
		}
	}
	// replays run side by side, but not all at once: a hook that a recording shows
	// under a one-second timeout (SessionEnd) is killed on a machine too loaded to start it
	slots := make(chan struct{}, 4)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			slots <- struct{}{}
			defer func() { <-slots }()
			diff, err := codexreplay.Run(mockBinary, filepath.Join(runsDir, name))
			var unbuildable *codexreplay.Unbuildable
			reason, listed := notReplaying[name]
			switch {
			case errors.As(err, &unbuildable) && listed:
				t.Skipf("not replaying: %s", reason)
			case errors.As(err, &unbuildable):
				t.Errorf("not replayed (%v): list it in notReplaying with the reason, or extend the adapter", err)
			case err != nil:
				t.Error(err)
			case diff == "" && listed && strings.HasPrefix(reason, "flaky:"):
			case diff == "" && listed:
				t.Errorf("replays green: remove it from notReplaying (was: %s)", reason)
			case diff != "" && listed:
				t.Skipf("not replaying: %s\n%s", reason, diff)
			case diff != "":
				t.Error(diff)
			}
		})
	}
}
