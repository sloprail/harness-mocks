package runner

// agentIsolation is the isolation an Agent call runs with: the call's own, else
// "worktree" for a custom sub-agent whose frontmatter sets `isolation: worktree`
// (it always runs in its own worktree).
//
// sr:docs https://code.claude.com/docs/en/worktrees#isolate-subagents-with-worktrees
func agentIsolation(cfg Config, in agentToolInput) string {
	if in.Isolation == "" && in.SubagentType != "" && definitionField(cfg, in.SubagentType, "isolation") == "worktree" {
		return "worktree"
	}
	return in.Isolation
}
