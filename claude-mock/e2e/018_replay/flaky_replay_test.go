package e2e

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// A flaky: entry is replayed flakyRuns times and passes when one run is green.
func TestReplayUntilGreenPassesOnALaterGreenRun(t *testing.T) {
	calls := 0
	diff, err := replayUntilGreen(func() (string, error) {
		calls++
		if calls < flakyRuns {
			return "differs", nil
		}
		return "", nil
	}, flakyRuns)
	require.NoError(t, err)
	require.Empty(t, diff)
	require.Equal(t, flakyRuns, calls)
}

// It stops at the first green run.
func TestReplayUntilGreenStopsAtTheFirstGreenRun(t *testing.T) {
	calls := 0
	_, err := replayUntilGreen(func() (string, error) {
		calls++
		return "", nil
	}, flakyRuns)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

// A run that is never green is returned as a failure after all its runs, never skipped.
func TestReplayUntilGreenFailsWhenNeverGreen(t *testing.T) {
	calls := 0
	diff, err := replayUntilGreen(func() (string, error) {
		calls++
		return "differs", nil
	}, flakyRuns)
	require.NoError(t, err)
	require.Equal(t, "differs", diff)
	require.Equal(t, flakyRuns, calls)

	calls = 0
	_, err = replayUntilGreen(func() (string, error) {
		calls++
		return "", errors.New("unbuildable")
	}, flakyRuns)
	require.Error(t, err)
	require.Equal(t, flakyRuns, calls)
}
