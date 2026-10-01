package tools

import "errors"

// GlobLimit is how many files a Glob returns (docs, Glob tool behavior).
const GlobLimit = 100

// BenignExit1 are the commands whose exit status 1 is a valid result, not a
// failure: they only report no match or a difference (docs, Bash tool behavior).
var BenignExit1 = []string{"grep", "rg", "egrep", "fgrep", "find", "diff", "test", "[", "git diff", "git grep"}

// ReadOutcome is what a read of a file comes to.
type ReadOutcome int

const (
	// ReadLines: the selected lines are the result.
	ReadLines ReadOutcome = iota
	// ReadEmpty: the file exists but has no content.
	ReadEmpty
	// ReadPastEnd: the offset is past the file's last line.
	ReadPastEnd
)

// OutcomeOf is which of the outcomes a read of content, giving v, comes to.
func (v View) OutcomeOf(content string) ReadOutcome {
	switch {
	case content == "":
		return ReadEmpty
	case v.NumLines == 0:
		return ReadPastEnd
	}
	return ReadLines
}

// Refusal is an Edit that cannot be acted on, whatever hooks think of it.
type Refusal struct {
	// Absent: the string is not in the file; otherwise it is ambiguous.
	Absent bool
	// Matches is how many times the string appears.
	Matches int
}

// RefusedEdit is whether an Edit of content is refused before any hook sees
// it: its string is absent, or ambiguous without replaceAll.
func RefusedEdit(content, old string, replaceAll bool) (Refusal, bool) {
	_, n, err := Edit(content, old, "", replaceAll)
	switch {
	case errors.Is(err, ErrNoMatch):
		return Refusal{Absent: true, Matches: n}, true
	case errors.Is(err, ErrAmbiguous):
		return Refusal{Matches: n}, true
	}
	return Refusal{}, false
}
