// Package e2etest provides shared helpers for claude-mock e2e tests.
package e2etest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// MockHangLimit is how long one mock run may take before RunInDir treats it as hung.
const MockHangLimit = 10 * time.Minute

// MockBinaryPath is the a10n-claude-mock binary path, set by Main.
var MockBinaryPath string

// Main resolves the binary path and runs all tests.
// Respects the A10N_CLAUDE_MOCK_TEST_BINARY env override; otherwise builds fresh.
func Main(m *testing.M) {
	tmp, err := os.MkdirTemp("", "a10n-claude-mock-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: create tmp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	if prebuilt := os.Getenv("A10N_CLAUDE_MOCK_TEST_BINARY"); prebuilt != "" {
		MockBinaryPath = prebuilt
	} else {
		gomodOut, err := exec.Command("go", "env", "GOMOD").Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e: go env GOMOD: %v\n", err)
			os.Exit(1)
		}
		moduleRoot := filepath.Dir(strings.TrimSpace(string(gomodOut)))
		MockBinaryPath = filepath.Join(tmp, "a10n-claude-mock")
		buildCmd := exec.Command("go", "build", "-o", MockBinaryPath, "./claude-mock")
		buildCmd.Dir = moduleRoot
		if out, err := buildCmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: build a10n-claude-mock failed: %v\n%s\n", err, out)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// Run invokes the mock binary with args and returns combined output plus exit code.
func Run(t *testing.T, args ...string) (string, int) {
	t.Helper()
	return RunInDir(t, t.TempDir(), nil, args...)
}

// RunWithScript invokes the mock with --script pointing at a temp script written
// from content, plus any extra args.
func RunWithScript(t *testing.T, scriptContent string, extraArgs ...string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "scenario.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	args := append([]string{"--script", scriptPath}, extraArgs...)
	return RunInDir(t, dir, nil, args...)
}

// SharedPluginCacheDir is the fixed plugin cache directory reused across all
// mock e2e tests so that plugins (which may be git-cloned) are not re-fetched
// on every test invocation.  It mirrors the default used by pluginCacheDir()
// in the hooks package.
//
// sr:docs https://code.claude.com/docs/en/env-vars#environment-variables (CLAUDE_CODE_PLUGIN_CACHE_DIR)
const SharedPluginCacheDir = "/tmp/a10n-mock-plugins"

// RunInDir invokes the mock binary from dir with optional extra env and args.
// CLAUDE_CODE_PLUGIN_CACHE_DIR is always injected so every test run shares the
// same plugin cache and avoids re-cloning plugins on each test. The Claude
// config dir (where transcripts live) and the temp root (where background task
// output lives) default to directories under dir, so no two tests share
// sessions through the mock's global defaults; a test's --config-dir or env
// still wins.
func RunInDir(t *testing.T, dir string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(MockBinaryPath, args...)
	cmd.Dir = dir
	baseEnv := append(os.Environ(), "CLAUDE_CODE_PLUGIN_CACHE_DIR="+SharedPluginCacheDir,
		"CLAUDE_CONFIG_DIR="+filepath.Join(dir, ".claude-config"),
		"CLAUDE_CODE_TMPDIR="+filepath.Join(dir, ".claude-tmp"))
	if len(env) > 0 {
		cmd.Env = append(baseEnv, env...)
	} else {
		cmd.Env = baseEnv
	}
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.WaitDelay = 10 * time.Second // a descendant that keeps the pipe open must not hold the test either
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mock: %v", err)
	}
	// A mock that never exits would hold the whole package until go test's own timeout, with a dump of the
	// test binary only. SIGQUIT makes the mock dump its own goroutines into the captured output instead.
	var hung atomic.Bool
	timer := time.AfterFunc(MockHangLimit, func() {
		hung.Store(true)
		_ = cmd.Process.Signal(syscall.SIGQUIT)
	})
	_ = cmd.Wait()
	timer.Stop()
	if hung.Load() {
		t.Errorf("the mock was still running after %s and was sent SIGQUIT; its goroutine dump is in the output:\n%s", MockHangLimit, out.String())
	}
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	return out.String(), code
}

// WriteScript writes a shell script to a temp file and returns its path.
func WriteScript(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mock-scenario-*.sh")
	if err != nil {
		t.Fatalf("create script: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write script: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close script: %v", err)
	}
	if err := os.Chmod(f.Name(), 0o755); err != nil {
		t.Fatalf("chmod script: %v", err)
	}
	return f.Name()
}
