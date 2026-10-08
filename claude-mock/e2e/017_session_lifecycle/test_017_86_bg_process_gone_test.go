package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A background shell the run terminates is really gone: a command that would
// write a marker after the grace is over never writes it, even once its sleep
// would have ended (headless#background-tasks-at-exit; runs/bgbash).
// sr:proves background-bash-reaped-at-exit/claude
func TestT017_86_ATerminatedBackgroundShellIsGone(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	sc := script(t, dir, "s",
		toolUse("bg", "Bash", `{"command":"sleep 8; touch `+marker+`","description":"late marker","run_in_background":true}`),
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"LAUNCHED @MARK@"}]}}`,
	)
	started := time.Now()
	out, code := runInDir(t, dir, nil, "--script", sc, "--session-id", "gone-1",
		"--project-dir", dir, "--config-dir", filepath.Join(dir, "config"), "-p", "hello")
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, `"status":"stopped"`)
	assert.Less(t, time.Since(started), 7*time.Second, "the run ended in the grace, not when the command would have")
	time.Sleep(5 * time.Second) // past the moment the sleep would have ended
	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "the stopped command's shell is gone: it wrote its marker")
}
