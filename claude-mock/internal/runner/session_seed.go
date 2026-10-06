package runner

import (
	"os"
)

// preambleTypes are the no-uuid preamble record types real Claude Code opens a
// transcript with — bookkeeping about the session rather than anything that happened
// in it. A consumer skips them (they carry no uuid and cannot join a chain) but counts
// their physical lines. Kept as a set so isPreambleType can recognise one already on
// disk (the re-entry idempotency guard).
var preambleTypes = map[string]bool{
	"custom-title":    true,
	"ai-title":        true,
	"mode":            true,
	"last-prompt":     true,
	"queue-operation": true,
	"pr-link":         true,
}

// mockPreambleRecords returns the fixed block of no-uuid preamble records the mock
// writes at the head of every FRESH transcript, so its file opens the way a real
// Claude Code session file does. Real transcripts open with these bookkeeping records
// (custom-title / mode / last-prompt / …); each carries NO uuid and NO parentUuid, so a
// reader counts the physical line but skips it as an entry. The mock previously wrote
// none — a fidelity gap (a real transcript opens with them; the mock's opened straight
// at the user prompt) — which this closes.
//
// Three representative shapes, verified byte-for-shape against a real
// ~/.claude/projects/<proj>/<session>.jsonl:
//
//	{"type":"custom-title","customTitle":"…","sessionId":"…"}
//	{"type":"mode","mode":"normal","sessionId":"…"}
//	{"type":"last-prompt","lastPrompt":"…","leafUuid":"…","sessionId":"…"}
//
// The content is deterministic — this is a mock, and nothing downstream reads the
// bookkeeping VALUES; only the "counted-but-skipped no-uuid line" shape matters. The
// sessionId is the run's own so the records look like they belong to this session.
func mockPreambleRecords(sessionID, name string) [][]byte {
	title := "a mock session"
	if name != "" {
		title = name
	}
	recs := []map[string]any{
		{"type": "custom-title", "customTitle": title, "sessionId": sessionID},
		{"type": "mode", "mode": "normal", "sessionId": sessionID},
		{"type": "last-prompt", "lastPrompt": "", "leafUuid": "", "sessionId": sessionID},
	}
	if name != "" {
		// a session started with --name carries its name, which a later --resume finds it by
		// (recorded: runs/resume-name)
		recs = append(recs[:1], append([]map[string]any{{"type": "agent-name", "agentName": name, "sessionId": sessionID}}, recs[1:]...)...)
	}
	var out [][]byte
	for _, r := range recs {
		if line, err := marshalRecord(r); err == nil {
			out = append(out, line)
		}
	}
	return out
}

// seedPreamble ensures the FRESH transcript opens with the no-uuid preamble records a
// real Claude Code session file does, sitting AHEAD of the root prompt record.
//
// # Why it prepends rather than just writes
//
// The root may already be on disk when this runs. Two callers seed it: the mock's own
// seedRootPromptTranscript (which just ran, for a mock-owned fresh session) and the
// sloprail HARNESS, which pre-seeds an identical root record into the file before it
// launches the mock at all. In both cases the file's first record is the root by the
// time this runs, and the preamble has to land BEFORE it — so this reads the existing
// content and rewrites the file as [preamble…, existing…]. The order the head ends up
// in is therefore [preamble…, root, conversation…] regardless of who wrote the root.
//
// # Idempotency
//
// It is a no-op when the transcript already opens with a preamble record (a re-entry
// that re-opened a file this already processed) — so the block is written exactly once.
// An empty file gets just the preamble (a later root append then follows); but in
// practice the root is already present, since fresh runs seed it first.
//
// # Safety with the open append handle
//
// f is opened O_APPEND. Rewriting the file's content with os.WriteFile (truncate+write)
// does not invalidate the fd; a subsequent O_APPEND write seeks to the true end at write
// time — after the prepended head — so the streamed records still land in order. Best-
// effort throughout: any read/write error leaves the transcript as-is (session
// persistence must never fail the run), matching seedRootPromptTranscript.
func seedPreamble(f *os.File, sessionID, name string) {
	if f == nil {
		return
	}
	existing, err := os.ReadFile(f.Name())
	if err != nil {
		return
	}
	if transcriptHasPreamble(existing) {
		return // already opened with a preamble — don't write it twice
	}
	var buf []byte
	for _, line := range mockPreambleRecords(sessionID, name) {
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	buf = append(buf, existing...)
	_ = os.WriteFile(f.Name(), buf, 0o644)
}
