package toolexec

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
)

// The environment a SessionStart hook persists: it writes shell lines (exports) to the file named in its
// CLAUDE_ENV_FILE, <config dir>/session-env/<session>/sessionstart-hook-<n>.sh, and every Bash command of
// the session then runs with them in effect (recorded: snapshots/runs/env-file-persist).

var configDir atomic.Value // string

// SetConfigDir names the config directory the session's env files are under.
func SetConfigDir(dir string) { configDir.Store(dir) }

// SessionEnvFiles are the env files the session's hooks wrote, in hook order.
func SessionEnvFiles(sessionID string) []string {
	dir, _ := configDir.Load().(string)
	if dir == "" || sessionID == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(dir, "session-env", sessionID, "sessionstart-hook-*.sh"))
	sort.Slice(files, func(i, j int) bool {
		return len(files[i]) < len(files[j]) || len(files[i]) == len(files[j]) && files[i] < files[j]
	})
	return files
}

// WithSessionEnv is a Bash command that first sources the session's env files.
func WithSessionEnv(sessionID, command string) string {
	var b strings.Builder
	for _, f := range SessionEnvFiles(sessionID) {
		if _, err := os.Stat(f); err == nil {
			b.WriteString(". '" + strings.ReplaceAll(f, "'", `'\''`) + "'\n")
		}
	}
	return b.String() + command
}
