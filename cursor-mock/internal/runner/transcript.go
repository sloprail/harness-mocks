package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// transcript is the conversation's record file, in the layout Cursor keeps
// (recorded: runs/*/samples/*/transcript): one JSON line per message, a user
// message first (opening with the empty <timestamp/> element, then the
// <user_query> element, as recorded), assistant messages with their text and
// tool calls, and a closing turn_ended line. It is what the scenario script reads as
// A10N_MOCK_SESSION_FILE.
type transcript struct {
	path string
	// said is what the agent said since the last record: a step's text and its
	// call are one record, and the text of a step that makes no call is a record
	// of its own at the end of the turn (recorded: runs/tool-failure).
	said []map[string]any
}

var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// newTranscript names the session's file under <home>/.cursor/projects/<project>/
// agent-transcripts/<session>/, where <project> is the workspace path (symlinks
// resolved by the entrypoint) with every non-alphanumeric character as "-". The
// file itself is written with the first record, after the start hook has run.
//
// sr:provides session-transcript-file/cursor
// sr:docs https://cursor.com/docs/hooks#common-schema
func newTranscript(home, dir, session string) (*transcript, error) {
	project := nonAlnum.ReplaceAllString(strings.TrimPrefix(dir, "/"), "-")
	d := filepath.Join(home, ".cursor", "projects", project, "agent-transcripts", session)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return nil, err
	}
	return &transcript{path: filepath.Join(d, session+".jsonl")}, nil
}

func (t *transcript) add(line any) {
	b, _ := json.Marshal(line)
	if f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		_, _ = f.Write(append(b, '\n'))
		_ = f.Close()
	}
}

func (t *transcript) message(role string, blocks ...map[string]any) {
	t.add(map[string]any{"role": role, "message": map[string]any{"content": blocks}})
}

func (t *transcript) user(prompt string) {
	t.flushText()
	t.message("user", map[string]any{"type": "text", "text": "<timestamp/>\n<user_query>\n" + prompt + "\n</user_query>"})
}

func (t *transcript) text(s string) {
	t.said = append(t.said, map[string]any{"type": "text", "text": s})
}

func (t *transcript) toolUse(name string, input map[string]any) {
	t.message("assistant", append(t.said, map[string]any{"type": "tool_use", "name": name, "input": input})...)
	t.said = nil
}

// flushText writes what the agent said, with no call after it, as a record.
func (t *transcript) flushText() {
	if len(t.said) > 0 {
		t.message("assistant", t.said...)
		t.said = nil
	}
}

func (t *transcript) end() {
	t.flushText()
	t.add(map[string]any{"type": "turn_ended", "status": "success"})
}

func (t *transcript) exists() bool { _, err := os.Stat(t.path); return err == nil }
