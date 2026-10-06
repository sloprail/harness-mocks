package replay

// Step is what the agent was asked after the run's first prompt, in a run of several
// steps (a session resumed, one forked), and what it did.
type Step struct {
	Prompt string
	Agent  Agent
}

// Recording is a recorded run in unified form.
type Recording struct {
	Dir    string            // the recorded run: an adapter reads what it needs of it here
	Prompt string            // what the agent was asked
	Setup  map[string]string // the run's own setup, by name: opaque to the core
	Agent  Agent
	// Later are the steps after the first, in order, each a run of the harness of its
	// own over the same home; empty for a run of one step.
	Later []Step
}

// Observed is what a run left that is compared, normalised: one line per
// event, in an order that is the behaviour.
type Observed struct {
	Events []string
	Hooks  []string
	// Exits are how each step ended, one line per step ("exit 0"): a step the
	// harness refuses, with a status that is not 0, is behaviour too.
	Exits []string
}
