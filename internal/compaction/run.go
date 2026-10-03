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
	// AfterStops fires the hook after the compaction in place of After, and
	// returns true when it stopped what follows: the session-start hook then
	// does not fire.
	AfterStops func() (stop bool)
	// ResumeLast puts the session-start hook after the hook after the
	// compaction, instead of before it.
	ResumeLast bool
	// Command records the command a manual compaction is, once everything else
	// has run.
	Command func()
}

// Result is how a compaction went.
type Result struct {
	// Happened is whether the compaction was made: not when the hook before it
	// stopped it.
	Happened bool
	// Stopped is whether a hook stopped it, before the compaction or after.
	Stopped bool
}

// Run compacts the session: the hook before it (which can stop it), the
// summarizer (manual only), the boundary, the summary, the session-start hook,
// the hook after it, and for a manual compaction the command's own records. It
// reports whether the compaction happened.
func Run(manual bool, s Steps) bool { return Do(manual, s).Happened }

// Do is Run, reporting also whether a hook stopped it: a stop after the
// compaction leaves it made and skips the session-start hook that follows.
//
// sr:capability manual-compaction
func Do(manual bool, s Steps) Result {
	if s.Before != nil && s.Before() {
		return Result{Stopped: true}
	}
	if manual && s.Summarizer != nil {
		s.Summarizer()
	}
	s.Boundary()
	s.Summary()
	if s.Resume != nil && !s.ResumeLast {
		s.Resume()
	}
	stopped := false
	switch {
	case s.AfterStops != nil:
		stopped = s.AfterStops()
	case s.After != nil:
		s.After()
	}
	if s.Resume != nil && s.ResumeLast && !stopped {
		s.Resume()
	}
	if manual && s.Command != nil {
		s.Command()
	}
	return Result{Happened: true, Stopped: stopped}
}
