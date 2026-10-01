package session

import "github.com/sloprail/harness-mocks/internal/transcript"

// Phase is a point in a session's life at which a harness fires a hook.
type Phase int

const (
	// Start is the beginning of a session.
	Start Phase = iota
	// End is the end of a session.
	End
)

// NoConversationError is a resume naming a session that has no transcript.
type NoConversationError struct {
	SessionID string
	// Fire is the phases whose hooks still fire when the resume fails: the end
	// of the session, and never its start.
	Fire []Phase
}

func (e *NoConversationError) Error() string { return "no conversation with session " + e.SessionID }

// Resumed is a session continued from its transcript.
type Resumed struct {
	// Path is the existing transcript, where the session's records go, wherever
	// it was begun.
	Path string
	// Reported is the transcript path the harness reports: under the project
	// directory of the directory the session is resumed in, which holds no file
	// when the session began elsewhere.
	Reported string
}

// Resume continues session id from its existing transcript, or fails with a
// *NoConversationError when there is none.
func Resume(l transcript.Layout, configDir, cwd, id string) (Resumed, error) {
	path := Find(l, configDir, cwd, id)
	if path == "" {
		return Resumed{}, &NoConversationError{SessionID: id, Fire: []Phase{End}}
	}
	return Resumed{Path: path, Reported: l.FilePath(configDir, cwd, id)}, nil
}
