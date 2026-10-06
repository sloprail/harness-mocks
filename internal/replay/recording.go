package replay

// Recording is a recorded run in unified form.
type Recording struct {
	Dir    string            // the recorded run: an adapter reads what it needs of it here
	Prompt string            // what the agent was asked
	Setup  map[string]string // the run's own setup, by name: opaque to the core
	Agent  Agent
	// Then are the later runs of the harness against the same session store (a resume, a fork),
	// in order, after the first.
	Then []Step
}

// Step is a later run of the harness against the store an earlier one left: what it was asked, the
// words it was given (a resume or a fork of the first session; opaque to the core), the directory it
// was run from (empty: the first run's), and what the model did in it.
type Step struct {
	Prompt string
	Args   []string
	Cwd    string
	Agent  Agent
}
