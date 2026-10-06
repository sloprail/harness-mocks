// Package childenv is Claude Code's names for the harness and the session in
// the environment of a process it starts (a hook, a Bash tool command).
package childenv

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Identity is what every child process of a Claude Code session sees, whatever
// it inherited (recorded: runs/nested-session-env, launched over decoys):
// CLAUDECODE=1 marks "running under Claude Code", CLAUDE_CODE_CHILD_SESSION=1
// that Claude Code itself launched the process, CLAUDE_PID the harness's own
// pid, CLAUDE_CODE_SESSION_ATTENDED=0 that no one attends a print-mode session,
// and CLAUDE_CODE_SESSION_ID names the active session (left out when unknown).
//
// inherited is the environment the child inherits, whose CLAUDE_CODE_TMPDIR (when it names one) roots the
// messaging socket.
//
// sr:provides subprocess-session-env/claude
// sr:docs https://code.claude.com/docs/en/env-vars (CLAUDECODE, CLAUDE_CODE_CHILD_SESSION, CLAUDE_CODE_SESSION_ID, CLAUDE_PID)
func Identity(sessionID string, inherited []string) map[string]string {
	return map[string]string{
		"CLAUDECODE":                   "1",
		"CLAUDE_CODE_CHILD_SESSION":    "1",
		"CLAUDE_CODE_SESSION_ATTENDED": "0",
		"CLAUDE_PID":                   strconv.Itoa(os.Getpid()),
		"CLAUDE_CODE_SESSION_ID":       sessionID,
		"CLAUDE_CODE_MESSAGING_SOCKET": filepath.Join(tempRoot(inherited), "cc-socks", strconv.Itoa(os.Getpid())+".sock"),
		"CLAUDE_CODE_MESSAGING_TOKEN":  token(),
	}
}

// Tool is what a Bash tool command sees: Identity, and CLAUDE_CODE_EXECPATH, the
// harness's own executable (a hook does not get it; recorded: runs/subprocess-session-env).
func Tool(sessionID string, inherited []string) map[string]string {
	ident := Identity(sessionID, inherited)
	if exe, err := os.Executable(); err == nil {
		ident["CLAUDE_CODE_EXECPATH"] = exe
	}
	return ident
}

// tempRoot is the harness's temp root: CLAUDE_CODE_TMPDIR when it was given one.
func tempRoot(inherited []string) string {
	for i := len(inherited) - 1; i >= 0; i-- {
		if dir, ok := strings.CutPrefix(inherited[i], "CLAUDE_CODE_TMPDIR="); ok && dir != "" {
			return dir
		}
	}
	return filepath.Join(os.TempDir(), "claude-"+strconv.Itoa(os.Getuid()))
}

var (
	tokenOnce  sync.Once
	tokenValue string
)

// token is the run's messaging secret, made once.
func token() string {
	tokenOnce.Do(func() {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		tokenValue = hex.EncodeToString(b)
	})
	return tokenValue
}

// Defaults are what a child sees only when Claude Code inherited none:
// CLAUDE_CODE_ENTRYPOINT is how the harness was started, which its launcher
// declares (an inherited value passes through, recorded in
// runs/nested-session-env); a print-mode run launched bare says sdk-cli
// (runs/subprocess-session-env). The docs do not name its values.
func Defaults() map[string]string {
	return map[string]string{"CLAUDE_CODE_ENTRYPOINT": "sdk-cli"}
}
