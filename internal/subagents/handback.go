package subagents

// HandBack reports whether the report a foreground sub-agent hands its parent
// carries the trailer naming the sub-agent and its usage. A foreground
// sub-agent blocks its parent until it has finished, and the parent gets its
// final report as the tool's result; types in noTrailer (the built-in
// read-only ones) get no trailer unless they ran in a worktree, which the
// trailer then names.
//
// sr:capability foreground-subagent-result
func HandBack(agentType, worktreePath string, noTrailer []string) (trailer bool) {
	if worktreePath != "" {
		return true
	}
	for _, t := range noTrailer {
		if t == agentType {
			return false
		}
	}
	return true
}
