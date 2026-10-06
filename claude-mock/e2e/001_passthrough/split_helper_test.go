package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sloprail/harness-mocks/claude-mock/e2etest"
	"github.com/sloprail/harness-mocks/internal/procexec"
)

// runSplit invokes the mock like e2etest.RunInDir, keeping its stdout and stderr apart.
func runSplit(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	base := append(os.Environ(), "CLAUDE_CODE_PLUGIN_CACHE_DIR="+e2etest.SharedPluginCacheDir,
		"CLAUDE_CONFIG_DIR="+filepath.Join(dir, ".claude-config"), "CLAUDE_CODE_TMPDIR="+filepath.Join(dir, ".claude-tmp"))
	res, _ := procexec.Run(context.Background(), procexec.Spec{Argv: append([]string{e2etest.MockBinaryPath}, args...), Dir: dir, Env: append(base, env...)})
	return string(res.Stdout), string(res.Stderr), res.ExitCode
}
