package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binary is the a10n-cursor-mock the tests drive: A10N_CURSOR_MOCK_TEST_BINARY,
// or one built fresh.
var binary string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "a10n-cursor-mock-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: %v\n", err)
		os.Exit(1)
	}
	if binary = os.Getenv("A10N_CURSOR_MOCK_TEST_BINARY"); binary == "" {
		out, err := exec.Command("go", "env", "GOMOD").Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e: go env GOMOD: %v\n", err)
			os.Exit(1)
		}
		root := filepath.Dir(strings.TrimSpace(string(out)))
		binary = filepath.Join(tmp, "a10n-cursor-mock")
		build := exec.Command("go", "build", "-o", binary, "./cursor-mock")
		build.Dir = root
		if out, err := build.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: build failed: %v\n%s\n", err, out)
			os.Exit(1)
		}
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}
