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
		// a run with no captured sample is a scenario authored and not recorded: no recording to replay
		if samples, _ := filepath.Glob(filepath.Join(d, "samples", "*")); len(samples) > 0 {
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
			diff, err := claudereplay.Run(e2etest.MockBinaryPath, filepath.Join(runsDir, name), os.Environ())
			var unbuildable *claudereplay.Unbuildable
			reason, listed := notReplaying[name]
			switch {
			case errors.As(err, &unbuildable) && strings.HasPrefix(unbuildable.Reason, claudereplay.RefusedPrefix):
				// a recording of what the mock refuses (fail-fast): there is no run to replay, so the mock is asked to
				// do what the run did, and must refuse it
				assertRefuses(t, filepath.Join(runsDir, name))
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

// assertRefuses runs the mock with the flags of the recording's setup/args and
// requires it to refuse them: a non-zero exit, and the flag named in its error.
func assertRefuses(t *testing.T, run string) {
	t.Helper()
	args, err := os.ReadFile(filepath.Join(run, "setup", "args"))
	require.NoError(t, err)
	flags := strings.Fields(string(args))
	require.NotEmpty(t, flags)
	script := filepath.Join(t.TempDir(), "s.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\necho RAN\n"), 0o755))
	out, code := e2etest.RunInDir(t, t.TempDir(), nil, append([]string{"--script", script, "--session-id", "refused-1", "-p", "--output-format", "stream-json"}, append(flags, "go")...)...)
	require.NotZero(t, code, out)
	require.Contains(t, out, flags[0], "the refusal names the flag")
	require.True(t, strings.Contains(out, "is not implemented by the mock") || strings.Contains(out, "unknown flag"), out)
	require.NotContains(t, out, "RAN")
}
