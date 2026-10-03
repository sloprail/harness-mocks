package session

import (
	"os"
	"strings"
)

// StartHook is what a harness's start hook does for one way a session begins.
type StartHook struct {
	// Fires is whether the hook fires at all.
	Fires bool
	// Source is how the session began as the hook's payload says it ("" when
	// the harness says nothing).
	Source string
}

// StartPolicy is a harness's start hook for a session that begins fresh and for
// one resumed by its id: what differs between harnesses (some report a source,
// some fire no hook on a resume) is its parameter, which of the two applies is
// the session's. Like ContinueTranscript and FindUnder it is part of the core of
// session-resume, whose marker is on Find.
type StartPolicy struct{ Fresh, Resumed StartHook }

// For is the start hook of a session that is resumed, or not.
func (p StartPolicy) For(resumed bool) StartHook {
	if resumed {
		return p.Resumed
	}
	return p.Fresh
}

// ContinueTranscript makes the file at path the one a resumed session goes on
// in: the record that closes it, when the harness writes one (isClosing), is
// taken off, so the new records follow the earlier ones and the file ends with
// the latest turn's. A path with no file holds no conversation to continue
// (the session was begun elsewhere) and is left alone.
func ContinueTranscript(path string, isClosing func(line string) bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if n := len(lines); n > 0 && isClosing(lines[n-1]) {
		return os.WriteFile(path, []byte(strings.Join(lines[:n-1], "\n")+"\n"), 0o644)
	}
	return nil
}
