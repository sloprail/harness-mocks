package turnloop

// contextOf is the context the agent has at a step: what the prompt hooks added
// and, when the harness adds more as the turn goes, what it has added so far.
func contextOf(p Params, prompt string) string {
	if p.Added == nil || p.Added() == "" {
		return prompt
	}
	if prompt == "" {
		return p.Added()
	}
	return prompt + "\n" + p.Added()
}
