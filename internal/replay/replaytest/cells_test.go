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

// sr:proves replay-fidelity
func TestAdjacentUsesOfADroppedKeyAreEachSeen(t *testing.T) {
	for _, text := range []string{"A model model here.", "A model/model here."} {
		if errs := run(t, cells(t, text, "Nothing.")); len(errs) != 2 {
			t.Fatalf("%q: want both uses reported, got %v", text, errs)
		}
	}
}

// sr:proves replay-fidelity
func TestAnExcuseExcusesOneOccurrenceOnly(t *testing.T) {
	root := cells(t, "It does not model the model.", "Nothing.")
	errs := run(t, root, Excuse{Cell: "c", Key: "model", Text: "does not model the model", Why: "x"})
	if len(errs) == 0 {
		t.Fatal("an excuse naming the key twice must be refused")
	}
	// the excuse for one occurrence leaves the other reported
	errs = run(t, root, Excuse{Cell: "c", Key: "model", Text: "does not model the", Why: "x"})
	if len(errs) != 1 || !strings.Contains(errs[0], `names "model"`) {
		t.Fatalf("the second use must stay reported, got %v", errs)
	}
}

// sr:proves replay-fidelity
func TestAnExcuseThatIsAmbiguousOrAbsentFails(t *testing.T) {
	root := cells(t, "It does not model a. It does not model b.", "Nothing.")
	if errs := run(t, root, Excuse{Cell: "c", Key: "model", Text: "does not model", Why: "x"}); len(errs) == 0 {
		t.Fatal("an excuse text occurring twice must be refused")
	}
	errs := run(t, root, Excuse{Cell: "c", Key: "model", Text: "does not model a", Why: "x"}, Excuse{Cell: "c", Key: "model", Text: "does not model b", Why: "x"}, Excuse{Cell: "c", Key: "model", Text: "words that are not there model", Why: "x"})
	if len(errs) != 1 || !strings.Contains(errs[0], "occurs 0 times") {
		t.Fatalf("only the absent excuse is refused, once, got %v", errs)
	}
}

// sr:proves replay-fidelity
func TestExcusesMatchWithoutRegardToCaseAndTheRunsPathsAreNotText(t *testing.T) {
	root := cells(t, "The mock does NOT Model it.", "Nothing.")
	if errs := run(t, root, Excuse{Cell: "c", Key: "MODEL", Text: "Does Not model it", Why: "x"}); len(errs) != 0 {
		t.Fatalf("case must not matter, got %v", errs)
	}
	dir := filepath.Join(root, "spec", "capabilities")
	doc := "statement: x\nproviders:\n  h:\n    runs:\n      - claude-mock/snapshots/runs/cwd\n"
	if err := os.WriteFile(filepath.Join(dir, "c.yaml"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if errs := run(t, root); len(errs) != 0 {
		t.Fatalf("a run's path is not what the cell says: %v", errs)
	}
}
