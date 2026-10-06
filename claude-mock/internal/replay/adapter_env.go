package replay

import (
	"fmt"
	"sort"
	"strings"
)

// env is the whole environment of the mock: what a capture gives claude (a
// home and a temp root of its own, the hook log; the config and plugin cache
// dirs are the mock's flags, as claude finds its own under the home), and the tools the hooks use,
// but no CLAUDE* or ANTHROPIC* variable of the session that runs the replay.
func (a Adapter) env(home, tmp, hookLog string) []string {
	env := []string{"HOME=" + home, "TMPDIR=" + tmp, "CLAUDE_CODE_TMPDIR=" + tmp, "HOOK_LOG=" + hookLog, "DISABLE_AUTOUPDATER=1",
		"GIT_AUTHOR_NAME=replay", "GIT_AUTHOR_EMAIL=replay@sloprail.invalid", "GIT_COMMITTER_NAME=replay", "GIT_COMMITTER_EMAIL=replay@sloprail.invalid"}
	for _, kv := range a.Environ {
		if k, _, _ := strings.Cut(kv, "="); k == "PATH" || k == "USER" || k == "LANG" || k == "TERM" {
			env = append(env, kv)
		}
	}
	return env
}

// readHooks reads a recording's hook payloads, each with the order of its keys.
func readHooks(path string) ([]map[string]any, error) {
	out, err := parseJSONL(readFile(path), true)
	if err != nil {
		return nil, unbuildable(fmt.Errorf("%s: %w", path, err))
	}
	return out, nil
}

// RunIDs are the task and agent ids the frames and payloads name: ids the
// harness makes up per run that, unlike a session's, have no pattern of their
// own, so the canonicalisation is told them. The keys that hold one are found
// by walking each object, not by searching its text.
func RunIDs(sets ...[]map[string]any) []string {
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if s, ok := e.(string); ok && s != "" && idKeys[k] {
					seen[s] = true
				}
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		}
	}
	for _, set := range sets {
		for _, o := range set {
			walk(o)
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	// longest first, so an id that holds another is renamed whole
	sort.Slice(ids, func(i, j int) bool { return len(ids[i]) > len(ids[j]) || len(ids[i]) == len(ids[j]) && ids[i] < ids[j] })
	return ids
}

// idKeys are the keys whose value is such an id.
var idKeys = map[string]bool{"task_id": true, "backgroundTaskId": true, "agent_id": true, "agentId": true}
