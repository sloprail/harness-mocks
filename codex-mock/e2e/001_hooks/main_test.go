package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mockBinary is the a10n-codex-mock the tests drive, built once.
var mockBinary string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "a10n-codex-mock-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}
	gomod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: go env GOMOD: %v\n", err)
		os.Exit(1)
	}
	mockBinary = filepath.Join(tmp, "a10n-codex-mock")
	build := exec.Command("go", "build", "-race", "-o", mockBinary, "./codex-mock")
	build.Dir = filepath.Dir(strings.TrimSpace(string(gomod)))
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build failed: %v\n%s\n", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}
