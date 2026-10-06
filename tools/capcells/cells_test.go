// Package capcells holds the tests of the capability-matrix rules: they run
// .sloprail/file-guard/capability-covered/covered.sh against throwaway trees
// whose capability file has one cell of each kind.
package capcells

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const header = `statement: A hook can block.
providers:
  claude:
    docs:
      - https://code.claude.com/docs/en/hooks#x
    runs:
      - claude-mock/snapshots/runs/r
`

// check builds a tree with capability "cap" (claude supported, plus the given
// other-mock cell text) and runs covered.sh on it. It returns the exit code and
// the combined output.
func check(t *testing.T, cell string, markers ...string) (int, string) {
	t.Helper()
	for _, bin := range []string{"jq", "yq", "git", "bash"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Fatalf("%s is not installed: the tests need it", bin)
		}
	}
	root := t.TempDir()
	write := func(p, s string) {
		t.Helper()
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("spec/capabilities/cap.yaml", header+cell)
	write("internal/cap/cap.go", "package cap\n\n// sr:capability cap\n")
	write("claude-mock/a.go", "package main\n\n// sr:provides cap/claude\n")
	write("claude-mock/a_test.go", "package main\n\n// sr:proves cap/claude\n")
	write("other-mock/a.go", "package main\n")
	for i, m := range markers {
		write("other-mock/m"+string(rune('a'+i))+".go", "package main\n\n"+m+"\n")
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "x"}} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	guard, err := filepath.Abs("../../.sloprail/file-guard/capability-covered")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(guard, "covered.sh"))
	cmd.Dir = guard
	cmd.Env = append(os.Environ(), "SR_TREE="+root, "SR_GUARDRAIL_DIR="+guard)
	cmd.Stdin = strings.NewReader(`{"event":{"kind":"Changeset"},"changeset":{"files":[]}}`)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return code, string(out)
}

func TestBareFalseIsRefused(t *testing.T) {
	code, out := check(t, "  other: false\n")
	if code == 0 || !strings.Contains(out, "bare false") {
		t.Fatalf("a bare false must be refused, got %d: %s", code, out)
	}
}

func TestFalseWithReasonAndDocsPasses(t *testing.T) {
	code, out := check(t, "  other:\n    supported: false\n    reason: no such hook\n    docs:\n      - https://example.com/hooks#events\n")
	if code != 0 {
		t.Fatalf("false with reason and docs must pass the script check, got %d: %s", code, out)
	}
}

func TestFalseWithRunsAlonePasses(t *testing.T) {
	code, out := check(t, "  other:\n    supported: false\n    reason: no such hook\n    runs:\n      - other-mock/snapshots/runs/r\n")
	if code != 0 {
		t.Fatalf("false with reason and a recorded run must pass the script check, got %d: %s", code, out)
	}
}

func TestFalseWithoutDocsOrRunsIsRefused(t *testing.T) {
	code, _ := check(t, "  other:\n    supported: false\n    reason: no such hook\n")
	if code == 0 {
		t.Fatal("false without docs or runs must be refused")
	}
}

func TestPendingIsAllowedButNotCoverage(t *testing.T) {
	code, out := check(t, "  other: pending\n")
	if code != 0 || !strings.Contains(out, "cap/other") {
		t.Fatalf("pending is allowed and listed, got %d: %s", code, out)
	}
	// a proves marker on a pending cell is not coverage: refused (the sr:provides side is
	// file-guard/capability-reconciled's)
	code, out = check(t, "  other: pending\n", "// sr:proves cap/other")
	if code == 0 || !strings.Contains(out, "not coverage") {
		t.Fatalf("a marker on a pending cell must be refused, got %d: %s", code, out)
	}
}

func TestSupportedCellStillNeedsAProvingTest(t *testing.T) {
	code, out := check(t, "  other:\n    docs:\n      - https://example.com/hooks#events\n    runs:\n      - other-mock/snapshots/runs/r\n")
	if code == 0 || !strings.Contains(out, "sr:proves") {
		t.Fatalf("a supported cell without a proving test must be refused, got %d: %s", code, out)
	}
}
