package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
	claudereplay "github.com/sloprail/harness-mocks/claude-mock/internal/replay"
)

// runsDir is where the recorded runs are.
var runsDir = filepath.Join("..", "..", "snapshots", "runs")

// Every recorded run, replayed by the mock's own replay (claudereplay.Run, which
// `a10n-claude-mock replay <run-dir>` runs too): the mock is given the run's own
// setup and the model's own turns, as a scenario script it generates in the
// format any scenario has, and its whole event stream and hook payloads are
// compared with what the real claude left. The table is the recording folders
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
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			reason, listed := notReplaying[name]
			flaky := listed && strings.HasPrefix(reason, "flaky:")
			run := func() (string, error) {
				return claudereplay.Run(e2etest.MockBinaryPath, filepath.Join(runsDir, name), os.Environ())
			}
			var diff string
			var err error
			if flaky {
				diff, err = replayUntilGreen(run, flakyRuns)
			} else {
				diff, err = run()
			}
			var unbuildable *claudereplay.Unbuildable
			switch {
			case flaky && (err != nil || diff != ""):
				t.Errorf("flaky entry never replayed green in %d runs (%s): triage it, or remove it from notReplaying: %v\n%s", flakyRuns, reason, err, diff)
			case flaky:
			case errors.As(err, &unbuildable) && listed:
				t.Skipf("not replaying: %s", reason)
			case errors.As(err, &unbuildable):
				t.Errorf("not replayed (%v): list it in notReplaying with the reason, or extend the adapter", err)
			case err != nil:
				t.Error(err)
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

// flakyRuns is how many times a "flaky:" entry is replayed: it is green in some runs and not in
// others, so it must be green in at least one, and it is never skipped outright.
const flakyRuns = 3

// replayUntilGreen runs a replay up to attempts times and returns the first green result, or the
// last run's when none is green.
func replayUntilGreen(run func() (string, error), attempts int) (string, error) {
	diff, err := run()
	for i := 1; i < attempts && (err != nil || diff != ""); i++ {
		diff, err = run()
	}
	return diff, err
}
