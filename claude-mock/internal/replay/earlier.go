package replay

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// An earlier run is a run of claude the setup's preparation makes, so that the run itself is one
// invocation that resumes, continues or forks what the earlier one left (prepare.sh of
// snapshots/runs/resume-continue). The replay runs the mock in its place, on the turns the
// recording's transcript holds for that run's prompt.

// earlierRun is one such run: the prompt it was given, the session id it was started under, and
// what the model did in it.
type earlierRun struct {
	Prompt string
	ID     string
	Agent  core.Agent
}

var (
	claudeWord  = regexp.MustCompile(`(^|[\s;&|(])claude(\s|$)`)
	claudePlug  = regexp.MustCompile(`(^|[\s;&|(])claude\s+plugin(\s|$)`)
	earlierID   = regexp.MustCompile(`--session-id\s+(\S+)`)
	earlierText = regexp.MustCompile(`'((?:[^']|'\\'')*)'\s*</dev/null`)
)

// earlierSpecs are the runs of claude a preparation script makes, in order, each with its prompt
// and session id (no agent yet). A `claude plugin` command is not one (the replay's own claude
// accepts those); a claude invocation the adapter cannot read is not replayed.
func earlierSpecs(prepare string) ([]stepSpec, error) {
	var out []stepSpec
	for _, line := range strings.Split(prepare, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !claudeWord.MatchString(line) || claudePlug.MatchString(line) {
			continue
		}
		id, text := earlierID.FindStringSubmatch(line), earlierText.FindStringSubmatch(line)
		if id == nil || text == nil || !strings.Contains(line, "claude -p ") {
			return nil, unbuildable(fmt.Errorf("the preparation runs claude in a way the adapter cannot read: %.80s", line))
		}
		out = append(out, stepSpec{prompt: strings.ReplaceAll(text[1], `'\''`, `'`), newID: id[1]})
	}
	return out, nil
}

// earlierJSON and earlierOf carry the earlier runs through the recording's opaque setup.
func earlierJSON(runs []earlierRun) string {
	if len(runs) == 0 {
		return ""
	}
	b, _ := json.Marshal(runs)
	return string(b)
}

func earlierOf(rec core.Recording) []earlierRun {
	var runs []earlierRun
	if text := rec.Setup["earlier"]; text != "" {
		_ = json.Unmarshal([]byte(text), &runs)
	}
	return runs
}
