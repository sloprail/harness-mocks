package e2e

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestT017_37_TheGraceBeforeTheShellIsStoppedIsAboutFiveSeconds: a `claude -p`
// run terminates a background shell about five seconds after the final result
// (headless#background-tasks-at-exit; snapshots/runs/bgbash, an 8s sleep stopped
// after the result): one that ends inside the grace completes, one still
// running past it is stopped, and the run lasts the grace, not the command.
// sr:proves background-bash-reaped-at-exit/claude
func TestT017_37_TheGraceBeforeTheShellIsStoppedIsAboutFiveSeconds(t *testing.T) {
	for _, c := range []struct {
		name, command, status string
		min, max              time.Duration
	}{
		{"inside", "sleep 3", "completed", 3 * time.Second, 5 * time.Second},
		{"past", "sleep 9", "stopped", 4500 * time.Millisecond, 7 * time.Second},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			sc := script(t, dir, "s",
				toolUse("bg", "Bash", `{"command":"`+c.command+`","description":"timed","run_in_background":true}`),
				`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED @MARK@"}]}}`,
			)
			started := time.Now()
			out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "grace-"+c.name,
				"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
			elapsed := time.Since(started)
			require.Equal(t, 0, code, out)
			assert.Contains(t, out, `"status":"`+c.status+`"`)
			assert.GreaterOrEqual(t, elapsed, c.min)
			assert.Less(t, elapsed, c.max)
		})
	}
}
