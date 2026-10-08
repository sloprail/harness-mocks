package runner

import (
	"strings"
	"testing"
	"time"
)

// A shell's file has a header that says what ran, and a footer once it succeeded
// (recorded: runs/task-stream-frames).
func TestTerminalFileHasTheHeaderAndFooterCursorWrites(t *testing.T) {
	at := time.Date(2026, 10, 3, 15, 16, 20, 258000000, time.UTC)
	h := terminalHeader(51562, "/w", "sh -c 'echo x'", "Echo x", "succeeded", at, 5446)
	want := "---\npid: 51562\ncwd: \"/w\"\ncommand: \"sh -c 'echo x'\"\ntitle: \"Echo x\"\nstatus: succeeded\nstarted_at: 2026-10-03T15:16:20.258Z\nrunning_for_ms: 5446     \n---\n"
	if h != want {
		t.Fatalf("header:\n%q\nwant\n%q", h, want)
	}
	if f := terminalFooter(5443, at.Add(5443*time.Millisecond)); !strings.HasPrefix(f, "\n---\nexit_code: 0\nelapsed_ms: 5443\nended_at: 2026-10-03T15:16:25.701Z\n---") {
		t.Fatalf("footer %q", f)
	}
	if strings.Contains(terminalHeader(1, "/w", "c", "", "running", at, 0), "title:") {
		t.Fatal("a shell with no description has no title line")
	}
}
