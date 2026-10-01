package compaction

// Steps are the parts of a compaction a harness supplies: what each does and
// how it is written is the harness's; the order and what can stop it are not.
type Steps struct {
	// Before fires the hook before the compaction; it returns true to stop it,
	// and nothing more happens.
	Before func() (stop bool)
	// Summarizer fires for the summarizer of a manual compaction.
	Summarizer func()
	// Boundary writes the boundary, then Summary the summary after it.
	Boundary, Summary func()
	// Resume fires the session-start hook that follows the summary.
	Resume func()
	// After fires the hook after the compaction.
	After func()
	// Command records the command a manual compaction is, once everything else
	// has run.
	Command func()
}

// Run compacts the session: the hook before it (which can stop it), the
// summarizer (manual only), the boundary, the summary, the session-start hook,
// the hook after it, and for a manual compaction the command's own records. It
// reports whether the compaction happened.
func Run(manual bool, s Steps) bool {
	if s.Before != nil && s.Before() {
		return false
	}
	if manual && s.Summarizer != nil {
		s.Summarizer()
	}
	s.Boundary()
	s.Summary()
	if s.Resume != nil {
		s.Resume()
	}
	if s.After != nil {
		s.After()
	}
	if manual && s.Command != nil {
		s.Command()
	}
	return true
}
