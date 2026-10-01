package tools

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestFilesReadAndWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "note.txt")
	if _, err := ReadFile(p); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reading a missing file: %v, want ErrNotFound", err)
	}
	if old, existed, err := WriteFile(p, "hi"); err != nil || existed || old != "" {
		t.Fatalf("first write = %q %v %v", old, existed, err)
	}
	if old, existed, err := WriteFile(p, "bye"); err != nil || !existed || old != "hi" {
		t.Fatalf("second write = %q %v %v, want the old contents back", old, existed, err)
	}
	if got, err := ReadFile(p); err != nil || got != "bye" {
		t.Fatalf("ReadFile = %q %v", got, err)
	}
}
