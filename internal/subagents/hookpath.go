package subagents

import (
	"regexp"
	"strings"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// hookWorktreePath is the path a worktree-creating hook returned: the last
// non-empty line of its stdout, with terminal escapes stripped (so a shell
// banner printed before it is ignored). ok is false when the hook printed none,
// which fails the creation.
func hookWorktreePath(stdout string) (path string, ok bool) {
	lines := strings.Split(ansi.ReplaceAllString(stdout, ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if p := strings.TrimSpace(lines[i]); p != "" {
			return p, true
		}
	}
	return "", false
}
