package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartPolicy_AppliesTheWayTheSessionBegan(t *testing.T) {
	p := StartPolicy{Fresh: StartHook{Fires: true, Source: "startup"}, Resumed: StartHook{}}
	if got := p.For(false); !got.Fires || got.Source != "startup" {
		t.Fatalf("fresh = %+v", got)
	}
	if got := p.For(true); got.Fires {
		t.Fatalf("resumed = %+v", got)
	}
}

func TestContinueTranscript_TakesOffTheClosingRecordOnly(t *testing.T) {
	closing := func(l string) bool { return strings.Contains(l, "turn_ended") }
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, []byte("{\"a\":1}\n{\"type\":\"turn_ended\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ContinueTranscript(path, closing); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "{\"a\":1}\n" {
		t.Fatalf("transcript = %q", b)
	}
	if err := ContinueTranscript(path, closing); err != nil { // nothing closing: unchanged
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "{\"a\":1}\n" {
		t.Fatalf("transcript = %q", b)
	}
	if err := ContinueTranscript(filepath.Join(t.TempDir(), "none"), closing); err != nil {
		t.Fatal(err)
	}
}
