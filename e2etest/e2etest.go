// Package e2etest provides shared helpers for claude-mock e2e tests.
package e2etest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
		buildCmd := exec.Command("go", "build", "-o", MockBinaryPath, "./services/claude-mock")
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

// RunInDir invokes the mock binary from dir with optional extra env and args.
func RunInDir(t *testing.T, dir string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(MockBinaryPath, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()
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
