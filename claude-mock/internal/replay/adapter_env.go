package replay

import (
	"fmt"
	"path/filepath"
	"strings"
)

// env is the whole environment of the mock: what a capture gives claude (a
// home and a temp root of its own, the hook log), and the tools the hooks use,
// but no CLAUDE* or ANTHROPIC* variable of the session that runs the replay.
func (a Adapter) env(home, tmp, hookLog string) []string {
	env := []string{"HOME=" + home, "TMPDIR=" + tmp, "CLAUDE_CODE_TMPDIR=" + tmp, "CLAUDE_CONFIG_DIR=" + filepath.Join(home, ".claude"),
		"HOOK_LOG=" + hookLog, "DISABLE_AUTOUPDATER=1",
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
