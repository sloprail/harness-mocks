package toolcall

// DeniedByRule is whether a deny rule of the settings refuses a command: a command that is exactly a
// denied one. The harness reads its rules in its own syntax and says what the refusal tells the agent;
// that a rule refuses the call, whatever the permission mode, so that the tool does not run, is this.
//
// sr:capability noninteractive-run
func DeniedByRule(denied []string, command string) bool {
	for _, d := range denied {
		if d == command {
			return true
		}
	}
	return false
}
