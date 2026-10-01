package toolexec

// probeHookVerdict decides what a hook command's exit status means: exit 2
// blocks, any other non-zero is a warning, 0 accepts (rule probe).
func probeHookVerdict(exitCode int) string {
	switch {
	case exitCode == 2:
		return "block"
	case exitCode != 0:
		return "warn"
	default:
		return "accept"
	}
}
