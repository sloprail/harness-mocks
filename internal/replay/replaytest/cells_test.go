package replaytest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recorder struct{ errs []string }

func (r *recorder) Helper()                   {}
func (r *recorder) Errorf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }
func (r *recorder) Fatalf(f string, a ...any) { r.errs = append(r.errs, fmt.Sprintf(f, a...)) }

func cells(t *testing.T, statement, deviation string) string {
	root := t.TempDir()
	dir := filepath.Join(root, "spec", "capabilities")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "statement: >-\n  " + statement + "\nproviders:\n  h:\n    docs:\n      - https://x/#cwd\n    reason: >-\n      " + deviation + "\n"
	if err := os.WriteFile(filepath.Join(dir, "c.yaml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func run(t *testing.T, root string, ex ...Excuse) []string {
	r := &recorder{}
	NoCellNames(r, root, "h", []string{"model", "cwd"}, ex)
	return r.errs
}

// sr:proves replay-fidelity
func TestAnUnexplainedNamingOfADroppedKeyFails(t *testing.T) {
	errs := run(t, cells(t, "The run keeps its CWD.", "Nothing."))
	if len(errs) != 1 || !strings.Contains(errs[0], `names "cwd"`) {
		t.Fatalf("a cell naming cwd (any case) must fail once, got %v", errs)
	}
}

func TestADocLinkIsNotWhatACellSays(t *testing.T) {
	if errs := run(t, cells(t, "Nothing.", "Nothing.")); len(errs) != 0 {
		t.Fatalf("the cwd in a docs link is not the cell's: %v", errs)
	}
}

// sr:proves replay-fidelity
func TestAnExcuseCoversOnlyItsOwnOccurrence(t *testing.T) {
	ex := Excuse{Cell: "c", Key: "model", Text: "the mock does not model it", Why: "a verb"}
	root := cells(t, "The mock does not model it.", "Nothing.")
	if errs := run(t, root); len(errs) != 1 {
		t.Fatalf("an occurrence with no excuse must fail once, got %v", errs)
	}
	if errs := run(t, root, ex); len(errs) != 0 {
		t.Fatalf("the excused occurrence fails: %v", errs)
	}
	// a second, real use of the word in the same cell is not excused by it
	root = cells(t, "The mock does not model it.", "The payload's model is kept.")
	errs := run(t, root, ex)
	if len(errs) != 1 || !strings.Contains(errs[0], `names "model"`) {
		t.Fatalf("a new use of the word must fail, got %v", errs)
	}
}

// sr:proves replay-fidelity
func TestAnExcuseNoCellNeedsFails(t *testing.T) {
	errs := run(t, cells(t, "Nothing.", "Nothing."), Excuse{Cell: "c", Key: "model", Text: "does not model", Why: "x"})
	if len(errs) != 1 || !strings.Contains(errs[0], "not needed") {
		t.Fatalf("got %v", errs)
	}
}
