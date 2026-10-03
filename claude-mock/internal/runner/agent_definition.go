package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/tasks"
)

// agentRunsInBackground reports whether an Agent call runs in the background:
// the call asks for it with run_in_background, or the sub-agent's definition
// does with `background: true` in its frontmatter, which keeps it in the
// background even when Claude asks for the foreground (recorded:
// snapshots/runs/bgagent-definition, a definition with `background: true`
// called with run_in_background false answers with the async receipt).
// What the definition adds is only the ask: whether a call runs in the
// background stays tasks.RunsInBackground's, with CLAUDE_CODE_DISABLE_BACKGROUND_TASKS.
//
// sr:provides background-agent/claude
func agentRunsInBackground(cfg Config, input json.RawMessage) bool {
	var in agentToolInput
	_ = json.Unmarshal(input, &in)
	asked := runsInBackground(input) || (in.SubagentType != "" && definitionBackground(cfg, in.SubagentType))
	return tasks.RunsInBackground(asked, cfg.BackgroundTasksDisabled)
}

// definitionBackground reports whether the sub-agent definition named name
// sets `background: true`.
func definitionBackground(cfg Config, name string) bool {
	return definitionField(cfg, name, "background") == "true"
}

// definitionField is a frontmatter field of the sub-agent definition named name
// (a markdown file with YAML frontmatter under .claude/agents of the project, or
// agents/ of the config directory), "" when it has none.
func definitionField(cfg Config, name, key string) string {
	dirs := []string{filepath.Join(cfg.Cwd, ".claude", "agents")}
	if cfg.ConfigDir != "" {
		dirs = append(dirs, filepath.Join(cfg.ConfigDir, "agents"))
	}
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			if fm := frontmatter(string(raw)); fm["name"] == name {
				return fm[key]
			}
		}
	}
	return ""
}

// frontmatter is the top-level `key: value` pairs between the leading `---`
// lines of a markdown file.
func frontmatter(text string) map[string]string {
	lines := strings.Split(text, "\n")
	out := map[string]string{}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return out
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok || strings.HasPrefix(l, " ") {
			continue
		}
		out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	return out
}
