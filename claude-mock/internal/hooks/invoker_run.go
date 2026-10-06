package hooks

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	corehooks "github.com/sloprail/harness-mocks/internal/hooks"
)

// runHandlers runs every handler of one event together, as Claude Code does
// (docs: "All matching hooks run in parallel"), and returns each handler's run
// in the order given, with what the command handlers did behind them. Only
// the command handlers carry an Outcome; an HTTP handler's slot is zero.
func (inv *Invoker) runHandlers(ctx context.Context, handlers []HandlerSpec, ev EventName, hookCwd string, payload []byte) ([]HandlerRun, []corehooks.Outcome) {
	runs := make([]HandlerRun, len(handlers))
	outs := make([]corehooks.Outcome, len(handlers))
	var cmds []corehooks.Command
	var at []int
	var wg sync.WaitGroup
	for i, h := range handlers {
		switch h.Type {
		case "command":
			if strings.TrimSpace(h.Command) == "" {
				continue
			}
			// sr:provides hook-command-handler/claude
			cmds = append(cmds, corehooks.Command{Line: strings.TrimSpace(h.Command), Args: h.Args.List(), Timeout: commandTimeout(h, ev), Env: h.env(inv.envFile(ev, len(cmds)))})
			at = append(at, i)
		case "http":
			wg.Add(1)
			go func() {
				defer wg.Done()
				runs[i] = inv.httpRun(ctx, h, ev, payload)
			}()
		default:
			slog.Debug("hooks: unsupported handler type", "type", h.Type)
		}
	}
	// Mirror the real claude CLI's hook environment (see NewInvoker): the
	// session's identity, whether or not a session id is known.
	// sr:provides hook-timeout/claude
	rt := corehooks.Runtime{Dir: hookDir(hookCwd, inv.cwd, inv.projectDir), Env: hookEnv(inv.sessionID, inv.projectDir), DefaultTimeout: defaultTimeout(ev), NewSession: true}
	for k, o := range corehooks.RunAll(ctx, cmds, payload, rt) {
		outs[at[k]] = o
		runs[at[k]] = commandRun(handlers[at[k]], ev, o)
	}
	wg.Wait()
	return runs, outs
}

// commandTimeout is a handler's own timeout. A session-end hook may raise its
// limit above the shared budget, but not beyond 60 seconds (docs, Common fields).
func commandTimeout(h HandlerSpec, ev EventName) time.Duration {
	limit := time.Duration(0)
	if ev == EventSessionEnd {
		limit = 60 * time.Second
	}
	return corehooks.CapTimeout(time.Duration(h.Timeout)*time.Second, limit)
}

// env is what only a plugin's hook is told: where the plugin is installed and
// where its persistent data lives (docs, Reference scripts by path).
func (h HandlerSpec) env(envFile string) []string {
	var env []string
	if envFile != "" {
		env = append(env, "CLAUDE_ENV_FILE="+envFile)
	}
	if h.PluginRoot == "" {
		return env
	}
	return append(env, "CLAUDE_PLUGIN_ROOT="+h.PluginRoot, "CLAUDE_PLUGIN_DATA="+h.PluginData)
}

// envFile is the path a SessionStart hook is told as CLAUDE_ENV_FILE, where the real harness lets it
// persist environment variables for the session: <config dir>/session-env/<session>/sessionstart-hook-<index>.sh
// (recorded: runs/subprocess-session-env). Only that event's hooks have one. The mock names the file and
// does not read it back: what a hook writes there reaches no later command.
func (inv *Invoker) envFile(ev EventName, index int) string {
	if ev != EventSessionStart || inv.configDir == "" || inv.sessionID == "" {
		return ""
	}
	return filepath.Join(inv.configDir, "session-env", inv.sessionID, fmt.Sprintf("sessionstart-hook-%d.sh", index))
}

// hookDir is where a command hook runs. Claude's order of candidates is the
// event's working directory, the directory the session started in, the project
// root, the home directory and the system temp directory (docs, Hook handler
// fields); core picks the first that exists.
func hookDir(cwd string, fallbacks ...string) string {
	home, _ := os.UserHomeDir()
	return corehooks.FirstDir(append(append([]string{cwd}, fallbacks...), home, os.TempDir())...)
}
