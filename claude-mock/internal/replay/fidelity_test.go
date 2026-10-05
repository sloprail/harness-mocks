package replay

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	core "github.com/sloprail/harness-mocks/internal/replay"

	"go.yaml.in/yaml/v3"
)

// claudeNotAbout are the places a capability cell's claude text uses a word the replay
// drops from the comparison, and why that is not the cell's behaviour being left
// out ("cell/key": reason). An entry the cells no longer need fails the test, so
// the list only shrinks. Every entry is prose or a banner, never a key the cell is about.
var claudeNotAbout = map[string]string{
	"background-agent/script": "prose: the scenario script the mock plays, not a script key",
	"session-fork/script":     "prose: the scenario script the mock plays, not a script key",
	"session-resume/script":   "prose: the scenario script the mock plays, not a script key",
}

// The claude replay drops a key from both sides only if no capability cell is
// about it: each cell's statement and its claude deviations (the text a cell says
// its behaviour in) are searched for every dropped key, bar the places claudeNotAbout
// explains.
// sr:proves replay-fidelity
func TestNoCellNamesWhatTheClaudeReplayDrops(t *testing.T) {
	keys := Rules("", "", nil).DropKeys
	sort.Strings(keys)
	cells, err := filepath.Glob(filepath.Join("..", "..", "..", "spec", "capabilities", "*.yaml"))
	if err != nil || len(cells) == 0 {
		t.Fatalf("no capability cells found: %v", err)
	}
	used := map[string]bool{}
	for _, path := range cells {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var cell struct {
			Statement string
			Providers map[string]any
		}
		if err := yaml.Unmarshal(raw, &cell); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		claude, _ := cell.Providers["claude"].(map[string]any)
		if claude == nil {
			continue
		}
		text := cell.Statement
		devs, _ := claude["deviations"].([]any)
		for _, d := range devs {
			if m, ok := d.(map[string]any); ok {
				s, _ := m["statement"].(string)
				text += " " + s
			}
		}
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		for _, k := range keys {
			if !regexp.MustCompile(`(^|[^A-Za-z_])` + regexp.QuoteMeta(k) + `($|[^A-Za-z_])`).MatchString(text) {
				continue
			}
			if _, ok := claudeNotAbout[name+"/"+k]; ok {
				used[name+"/"+k] = true
				continue
			}
			t.Errorf("the cell %s names %q, which the claude replay drops from the comparison: scrub it to a placeholder (the key stays), or explain in claudeNotAbout", name, k)
		}
	}
	for k := range claudeNotAbout {
		if !used[k] {
			t.Errorf("claudeNotAbout lists %s, which no cell needs: remove it", k)
		}
	}
}

// A value that differs per run and is about a cell's presence (a working
// directory, a transcript path, a duration) is replaced by a placeholder, the
// key stays: a payload that lacks it is a difference.
// sr:proves replay-fidelity
func TestPerRunValuesKeepTheirKey(t *testing.T) {
	repo, root := "/tmp/r/repo", "/tmp/r"
	payload := map[string]any{
		"cwd":                   repo,
		"transcript_path":       root + "/home/.claude/projects/rollout.jsonl",
		"agent_transcript_path": root + "/home/.claude/projects/agent.jsonl",
		"wall_time_seconds":     0.5,
		"hook_event_name":       "Stop",
	}
	line := core.New(Rules(repo, root, nil)).Lines([]map[string]any{payload})[0]
	var missing []string
	for _, k := range []string{"cwd", "transcript_path", "agent_transcript_path", "wall_time_seconds"} {
		if !strings.Contains(line, `"`+k+`":`) {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("dropped from the payload instead of scrubbed: %v (line %s)", missing, line)
	}
	if strings.Contains(line, repo) || strings.Contains(line, root) {
		t.Fatalf("a per-run path is not replaced by a placeholder: %s", line)
	}
}
