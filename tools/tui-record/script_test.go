package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A mode the program turns on (bracketed paste) is a token in the text, so a script can wait for
// the moment a TUI starts taking input.
func TestScreenShowsTheModesAProgramSets(t *testing.T) {
	s := NewScreen(func(string) {})
	s.Write([]byte("banner\x1b[?25l\x1b[?2004h more"))
	got, _ := s.Since(0)
	if got != "banner <?25l> <?2004h> more" {
		t.Fatalf("screen %q", got)
	}
}

// A prompt kept in a file beside the script is typed as written, less its last newline.
func TestTextComesFromAFileBesideTheScript(t *testing.T) {
	r := newRig(t)
	if err := os.WriteFile(filepath.Join(r.dir, "prompt.txt"), []byte("do the thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := r.script(t, "steps:\n  - send: {text_file: prompt.txt, keys: [enter]}\n")
	if got := sc.Steps[0].Send.Text; got != "do the thing" {
		t.Fatalf("text %q", got)
	}
}
