package tools

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestRunShellResultAndFailure(t *testing.T) {
	env := []string{"PATH=/usr/bin:/bin"}
	r, err := RunShell(context.Background(), Shell{Command: "echo out; echo err >&2", Env: env})
	if err != nil || r.Failed() || r.Output() != "out\nerr\n" {
		t.Fatalf("RunShell = %+v, %v", r, err)
	}
	r, err = RunShell(context.Background(), Shell{Command: "echo OOPS >&2; exit 3", Env: env, Dir: t.TempDir()})
	if err != nil || !r.Failed() || r.ExitCode != 3 || r.Stderr != "OOPS\n" {
		t.Fatalf("a failing command = %+v, %v", r, err)
	}
}

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
