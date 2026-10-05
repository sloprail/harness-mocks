package replay

import (
	"path/filepath"
	"strings"
	"testing"

	core "github.com/sloprail/harness-mocks/internal/replay"
	"github.com/sloprail/harness-mocks/internal/replay/replaytest"
)

// excuses are the occurrences of a dropped key's word in the cells' codex text
// that are prose, not the key (see replaytest.Excuse). Filled in below.
var excuses = []replaytest.Excuse{
	{Cell: "agent-input-validation", Key: "model", Text: "does not model the items form", Why: "prose: 'model' is a verb (the mock does not model it), not the payload's model key"},
	{Cell: "background-agent", Key: "script", Text: "driven by the scenario script it is given", Why: "prose: the scenario script the mock plays, not a script key"},
	{Cell: "background-bash-reaped-at-exit", Key: "model", Text: "the recorded model started its command", Why: "prose: the model is the LLM the mock stands in for, not the payload's model key"},
	{Cell: "background-bash-reaped-at-exit", Key: "script", Text: "through codex's script tool", Why: "prose: the scenario script the mock plays, not a script key"},
	{Cell: "background-bash", Key: "model", Text: "does not model polling a running command", Why: "prose: 'model' is a verb (the mock does not model it), not the payload's model key"},
	{Cell: "file-tools", Key: "model", Text: "does not model moving a file", Why: "prose: 'model' is a verb (the mock does not model it), not the payload's model key"},
	{Cell: "foreground-subagent-result", Key: "script", Text: "no wait_agent call of the script is modelled", Why: "prose: the scenario script the mock plays, not a script key"},
	{Cell: "manual-compaction", Key: "model", Text: "sends /compact to the model as ordinary prompt text", Why: "prose: the model is the LLM the mock stands in for, not the payload's model key"},
	{Cell: "manual-compaction", Key: "script", Text: "a scenario script requests the compaction", Why: "prose: the scenario script the mock plays, not a script key"},
	{Cell: "noninteractive-run", Key: "model", Text: "it has no model, so the final message", Why: "prose: the model is the LLM the mock stands in for, not the payload's model key"},
	{Cell: "noninteractive-run", Key: "model", Text: "it calls no model and has no failing turn", Why: "prose: the model is the LLM the mock stands in for, not the payload's model key"},
	{Cell: "noninteractive-run", Key: "model", Text: "directory, model and session; the prompt", Why: "the text-mode banner printed on stderr, not the event stream's model key"},
	{Cell: "plugin-hooks", Key: "model", Text: "does not model a plugin's install", Why: "prose: 'model' is a verb (the mock does not model it), not the payload's model key"},
	{Cell: "session-fork", Key: "script", Text: "so a script cannot read the earlier conversation", Why: "prose: the scenario script the mock plays, not a script key"},
	{Cell: "session-start-hook", Key: "model", Text: "or model request after it", Why: "prose: the model is the LLM the mock stands in for, not the payload's model key"},
	{Cell: "subagent-lifecycle-hooks", Key: "model", Text: "the mock does not model. only the stop hook's", Why: "prose: 'model' is a verb (the mock does not model it), not the payload's model key"},
	{Cell: "task-stream-frames", Key: "model", Text: "the mock runs no model", Why: "prose: the model is the LLM the mock stands in for, not the payload's model key"},
	{Cell: "task-stream-frames", Key: "script", Text: "the scenario script the spawn_agent call names", Why: "the mock's own spawn_agent input that names a sub-agent's script, which the real tool has no such key for: the cell declares it as mock-only (mock-not-modeled)"},
}

// The codex replay drops a key from both sides only if no capability cell is
// about it: everything a cell says for codex is searched, whatever the case, for
// each dropped key, bar the occurrences excuses explains.
// sr:proves replay-fidelity
func TestNoCellNamesWhatTheCodexReplayDrops(t *testing.T) {
	replaytest.NoCellNames(t, filepath.Join("..", "..", ".."), "codex", Rules("", "").DropKeys, excuses)
}

// A value that differs per run and is about a cell's presence (a working
// directory, a transcript path, a duration) is replaced by a placeholder, the
// key stays: a payload that lacks it is a difference.
// sr:proves replay-fidelity
func TestPerRunValuesKeepTheirKey(t *testing.T) {
	repo, root := "/tmp/r/repo", "/tmp/r"
	payload := map[string]any{
		"cwd":                   repo,
		"transcript_path":       root + "/home/.codex/sessions/rollout.jsonl",
		"agent_transcript_path": root + "/home/.codex/sessions/agent.jsonl",
		"wall_time_seconds":     0.5,
		"hook_event_name":       "Stop",
	}
	line := core.New(Rules(repo, root)).Lines([]map[string]any{payload})[0]
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
