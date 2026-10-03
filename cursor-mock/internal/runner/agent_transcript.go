package runner

import (
	"os"
	"path/filepath"

	"github.com/sloprail/harness-mocks/internal/subagents"
)

// cursorSubagentLayout is where Cursor keeps a sub-agent's transcript: a
// directory and a file of its own, named by its own id, next to the session's
// directory under agent-transcripts, so it is a conversation like any other
// there and not a part of the session's. Cursor keeps no file that says what
// type of sub-agent it is, which call spawned it or how deep it is
// (recorded: runs/subagent-transcripts).
var cursorSubagentLayout = subagents.Layout{Ext: ".jsonl", OwnDir: true}

// newSubagentTranscript is the transcript of sub-agent id for the session whose
// transcript is parent: its own file, never the session's.
//
// sr:provides subagent-transcripts/cursor
func newSubagentTranscript(parent *transcript, id string) (*transcript, error) {
	t := &transcript{path: cursorSubagentLayout.Path(parent.path, id)}
	return t, os.MkdirAll(filepath.Dir(t.path), 0o755)
}
