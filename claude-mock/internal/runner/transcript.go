package runner

import (
	"os"

	coretranscript "github.com/sloprail/harness-mocks/internal/transcript"
)

// transcript is one run's handle on the session record it writes — the file,
// the chain through it, and the path the harness REPORTS for it, which are not
// always the same thing.
//
// # Why the file is opened lazily
//
// Real Claude Code does not write a fresh session's transcript before
// SessionStart. Measured across every real transcript on one machine: all 940
// `SessionStart:startup` runs of a hook that read its own transcript found no
// file, and in every one the transcript's origin record (its first parentless
// record) was the attachment recording that very SessionStart hook's result —
// written after the hook exited. When no SessionStart hook prints anything, no
// attachment is written and the origin is the user's prompt. So the mock does
// not create the file until the first record is written (coretranscript.Lazy),
// and the first record of a fresh session is whatever the harness writes first.
//
// # Reported path vs actual path
//
// A session resumed from a different working directory keeps appending to the
// transcript where it began, but real Claude Code reports transcript_path under
// the project directory of the directory it was resumed in — a file that does
// not exist. Measured: a session begun in a worktree and resumed from the main
// checkout got a SessionStart:resume payload naming
// <projects>/<main checkout>/<id>.jsonl, while that hook's own attachment and
// every later record went to <projects>/<worktree>/<id>.jsonl. The mock models
// it the same way: `path` is where records go, `reported` is what payloads say.
type transcript struct {
	path     string // where records are written
	reported string // what hook payloads carry as transcript_path
	lazy     *coretranscript.Lazy
	f        *os.File
	sw       *sessionWriter
	stamp    recordStamp

	// held is hook runs recorded but not written yet (holdHookRuns).
	held []heldRun
	// onFeedback is told the reason of each Stop hook feedback record written: the main thread's
	// stream carries it as a frame too (recorded: runs/cap).
	onFeedback func(reason string)
}

// openTranscript returns a handle on path. The file is opened now when it
// already exists (a resume, a nested sub-agent run, a caller that pre-seeded it)
// and on the first write otherwise.
//
// sr:provides session-transcript-file/claude
func openTranscript(path, reported string, stamp recordStamp) (*transcript, error) {
	lz, err := coretranscript.NewLazy(path, reported)
	if err != nil {
		return nil, err
	}
	t := &transcript{path: lz.Path, reported: lz.Reported, lazy: lz, stamp: stamp}
	if lz.Exists() {
		t.attach()
	}
	return t, nil
}

// attach opens the file for writing. A file this run creates opens with the
// preamble real Claude Code opens a main transcript with (not a sidechain's).
func (t *transcript) attach() {
	t.f = t.lazy.File(func() [][]byte {
		if t.stamp.IsSidechain {
			return nil
		}
		return mockPreambleRecords(t.stamp.SessionID)
	})
	if t.f == nil {
		return
	}
	t.sw = newSessionWriter(t.f)
	t.sw.stamp = t.stamp
}

// ensure opens the file for writing, writing the preamble first when this run
// is the one creating it.
func (t *transcript) ensure() {
	if t == nil || t.f != nil {
		return
	}
	t.attach()
}

// file is the open file, opening it if needed — for code that still speaks in
// *os.File (the script's A10N_MOCK_SESSION_FILE, the legacy seeders).
func (t *transcript) file() *os.File {
	t.ensure()
	return t.f
}

// Close closes the file if it was ever opened.
func (t *transcript) Close() {
	if t != nil {
		t.lazy.Close()
	}
}

// exists reports whether the file has been written.
func (t *transcript) exists() bool { return t.lazy.Exists() }

// persist writes one record, chained after the one before it.
func (t *transcript) persist(line []byte) {
	if t == nil {
		return
	}
	t.ensure()
	t.sw.persist(line)
}

// persistMap marshals rec and persists it.
func (t *transcript) persistMap(rec map[string]any) {
	if b, err := marshalRecord(rec); err == nil {
		t.persist(b)
	}
}

// lastUUID is the uuid of the last record written (or already on disk).
func (t *transcript) lastUUID() string {
	if t == nil || t.sw == nil {
		if t != nil && t.exists() {
			return lastRecordUUID(t.path)
		}
		return ""
	}
	return t.sw.lastUUID
}
