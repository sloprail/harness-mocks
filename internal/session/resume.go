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
//
// sr:capability session-resume-unknown
func Resume(l transcript.Layout, configDir, cwd, id string) (Resumed, error) {
	r, err := ResumeAt(Find(l, configDir, cwd, id), id, End)
	if err != nil {
		return Resumed{}, err
	}
	r.Reported = l.FilePath(configDir, cwd, id)
	return r, nil
}

// ResumeAt is the resume of session id whose transcript a harness found at
// path ("" when it holds none): the session, or a *NoConversationError whose
// Fire is the phases whose hooks still fire on that failure, which differ per
// harness (none at all, for one that fails before it starts a session).
func ResumeAt(path, id string, fire ...Phase) (Resumed, error) {
	if path == "" {
		return Resumed{}, &NoConversationError{SessionID: id, Fire: fire}
	}
	return Resumed{Path: path}, nil
}
