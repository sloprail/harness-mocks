package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// A background shell's output file is <terminals>/<shell id>.txt: a header that
// says what ran, the output, and once the command has succeeded a footer that says
// how it ended (recorded: runs/task-stream-frames, a read of the file after its
// shell ended). Only that ending is recorded: any other keeps the output alone.

// terminalHeader is the header of a shell's file: its process, where and what it
// ran, its title (the model's description of it, when it gave one), its status,
// when it started and how long it had run, padded to the width the file keeps so
// that it can be rewritten in place.
func terminalHeader(pid int, cwd, command, title, status string, started time.Time, runningMs int64) string {
	quote := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	var b strings.Builder
	fmt.Fprintf(&b, "---\npid: %d\ncwd: %s\ncommand: %s\n", pid, quote(cwd), quote(command))
	if title != "" {
		fmt.Fprintf(&b, "title: %s\n", quote(title))
	}
	fmt.Fprintf(&b, "status: %s\nstarted_at: %s\nrunning_for_ms: %-9d\n---\n", status, terminalTime(started), runningMs)
	return b.String()
}

// terminalTime is a time the way the file writes it: UTC, to the millisecond.
func terminalTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// terminalFooter is what ends the file of a shell that succeeded.
func terminalFooter(elapsedMs int64, ended time.Time) string {
	return fmt.Sprintf("\n---\nexit_code: 0\nelapsed_ms: %d\nended_at: %s\n---\n", elapsedMs, terminalTime(ended))
}

// finishTerminal is what ends a shell's file: the footer after the output of one
// that succeeded (the trailer), and the header rewritten with its pid and its
// final status (ended).
func (s *session) finishTerminal(path string, started time.Time, cwd, command, title string) (trailer func(int, bool) string, ended func(*tasks.Task)) {
	trailer = func(exit int, killed bool) string {
		if exit != 0 || killed {
			return ""
		}
		return terminalFooter(time.Since(started).Milliseconds(), time.Now())
	}
	ended = func(t *tasks.Task) {
		if t.ExitCode != 0 || t.Killed() {
			return
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		_, rest, ok := strings.Cut(strings.TrimPrefix(string(b), "---\n"), "\n---\n")
		if !ok {
			return
		}
		header := terminalHeader(t.Pid, cwd, command, title, "succeeded", started, time.Since(started).Milliseconds())
		_ = os.WriteFile(path, []byte(header+rest), 0o644)
	}
	return trailer, ended
}
