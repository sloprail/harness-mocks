package subagents

import "strings"

// Part of the foreground-subagent-result capability (its core marker is on HandBack).
//
// Class is the kind of work a tool call is, for the tally of a sub-agent's
// calls: the harness names which tools are which.
type Class int

const (
	// Other is any tool a harness does not class.
	Other Class = iota
	Read
	Search
	Shell
	Edit
	// Skipped calls (a sub-agent dispatching another) are not counted.
	Skipped
)

// Call is one tool call of a sub-agent: its tool, and for an edit the text it
// wrote and the text it replaced.
type Call struct {
	Tool         string
	Added, Taken string
}

// Counts is the tally of a sub-agent's calls.
type Counts struct {
	Read, Search, Shell, Edits, Other int
	LinesAdded, LinesRemoved          int
}

// Total is how many calls were counted.
func (c Counts) Total() int { return c.Read + c.Search + c.Shell + c.Edits + c.Other }

// Tally counts a sub-agent's calls by class (the harness's classes of its
// tools; a tool it does not class is Other), and the lines the edits add and
// remove.
func Tally(calls []Call, classes map[string]Class) Counts {
	var c Counts
	for _, call := range calls {
		switch classes[call.Tool] {
		case Read:
			c.Read++
		case Search:
			c.Search++
		case Shell:
			c.Shell++
		case Edit:
			c.Edits++
			c.LinesAdded += lines(call.Added)
			c.LinesRemoved += lines(call.Taken)
		case Skipped:
		default:
			c.Other++
		}
	}
	return c
}

// lines is how many lines s has; none for an empty string.
func lines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
