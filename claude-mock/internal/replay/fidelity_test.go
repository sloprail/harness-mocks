package replay

import (
	"path/filepath"
	"strings"
	"testing"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/replay/replaytest"
)

// excuses are the occurrences of a dropped key's word in the cells' claude text
// that are prose, not the key (see replaytest.Excuse). Filled in below.
var excuses = []replaytest.Excuse{
	{Cell: "background-agent", Key: "script", Text: "a mock sub-agent is a script run to its result", Why: "prose: the scenario script the mock plays, not a script key"},
	{Cell: "session-fork", Key: "script", Text: "are the scenario script's own in the mock", Why: "prose: the scenario script the mock plays, not a script key"},
	{Cell: "session-resume", Key: "script", Text: "is the scenario script's in the mock", Why: "prose: the scenario script the mock plays, not a script key"},
}

// The claude replay drops a key from both sides only if no capability cell is
// about it: everything a cell says for claude is searched, whatever the case, for
// each dropped key, bar the occurrences excuses explains.
// sr:proves replay-fidelity
func TestNoCellNamesWhatTheClaudeReplayDrops(t *testing.T) {
	replaytest.NoCellNames(t, filepath.Join("..", "..", ".."), "claude", Rules("", "", nil).DropKeys, excuses)
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
