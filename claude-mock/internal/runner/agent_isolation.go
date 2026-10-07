package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
)

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

// worktreeBaseRef is the `worktree.baseRef` setting a sub-agent's worktree is based on:
// "head" branches from the current HEAD, anything else ("fresh", the default) from the
// repository's default branch. The project's local settings win over its shared ones,
// and those over the user's.
//
// sr:docs https://code.claude.com/docs/en/worktrees#choose-the-base-branch
func worktreeBaseRef(cfg Config) string {
	files := []string{filepath.Join(cfg.Cwd, ".claude", "settings.local.json"), filepath.Join(cfg.Cwd, ".claude", "settings.json")}
	if cfg.ConfigDir != "" {
		files = append(files, filepath.Join(cfg.ConfigDir, "settings.json"))
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s struct {
			Worktree struct {
				BaseRef string `json:"baseRef"`
			} `json:"worktree"`
		}
		if json.Unmarshal(raw, &s) == nil && s.Worktree.BaseRef != "" {
			return s.Worktree.BaseRef
		}
	}
	return "fresh"
}
