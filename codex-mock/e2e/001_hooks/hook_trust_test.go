package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded runs runs/hook-trust-untrusted, runs/hook-trust-config, runs/project-hooks-trust-*,
// runs/disable-hooks, runs/ignore-user-config and runs/plugin-hook-env: Codex runs a non-managed hook
// only when it is trusted (its hash is the trusted_hash of its key in config.toml, or the run passes
// --dangerously-bypass-hook-trust), skipping an untrusted one without a word; the project layer's
// hooks.json loads only in a trusted project; --disable hooks loads none; --ignore-user-config
// leaves config.toml unread (its trust and its plugins), not hooks.json.

// the hashes Codex itself reported for these two hooks (runs/hook-trust-listed: its hooks/list answer,
// checked below): event pre_tool_use, matcher Bash, the command, no timeout (so 600), not async.
const (
	userHookHash    = "sha256:d978dfb3851e6eb6af88fd8323e578998edac8eb593f6b962c2ede394cbe6b41"
	projectHookHash = "sha256:fe1d1399af4be296a812de676629165e585a6b201058fb71959eb1bd91cc5d4d"

	trustMarker  = "#!/bin/sh\ncat >/dev/null\nprintf '{\"hook_ran\":\"%s\"}\\n' \"$1\" >>\"$HOOK_LOG\"\n"
	userHookJSON = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh user"}]}]}}`
	projHookJSON = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh project"}]}]}}`

	trustedUser    = "[hooks.state.\"{HOME}/hooks.json:pre_tool_use:0:0\"]\ntrusted_hash = \"" + userHookHash + "\"\n\n"
	trustedProject = "[hooks.state.\"{REPO}/.codex/hooks.json:pre_tool_use:0:0\"]\ntrusted_hash = \"" + projectHookHash + "\"\n\n"
	trustsRepo     = "[projects.\"{REPO}\"]\ntrust_level = \"trusted\"\n"
)

// The hashes the tests trust are the recorded ones.
func TestTheTrustedHashesAreTheRecordedOnes(t *testing.T) {
	list := readFile(t, filepath.Join(loadRecording(t, "hook-trust-listed").sample, "hooks-list.json"))
	assert.Contains(t, list, userHookHash)
	assert.Contains(t, list, projectHookHash)
}

// trustRun runs one Bash call under the user and project hooks above, and returns which hooks ran and the run.
func trustRun(t *testing.T, s scenario) ([]string, result) {
	t.Helper()
	s.HooksJSON, s.ProjectHooksJSON = userHookJSON, projHookJSON
	s.Files = map[string]string{"hook.sh": trustMarker}
	s.Script, s.Prompt, s.Env = callThenResult, "go", withCalls(t, "true")
	got := execMock(t, s)
	require.Equal(t, 0, got.Code, got.Stderr)
	return ranHooks(got.hookLog()), got
}

// A hook that is not trusted does not run, and the run says nothing of it.
// sr:proves hooks-all-matching-run/codex
func TestAnUntrustedHookIsSkippedSilently(t *testing.T) {
	ran, got := trustRun(t, scenario{Untrusted: true, Sandbox: "workspace-write"})
	assert.Empty(t, ran)
	for _, e := range got.stream() {
		item, _ := e["item"].(map[string]any)
		assert.NotEqual(t, "error", item["type"], "no notice: %v", e)
	}
	assert.Empty(t, got.Stderr)
}

// A hook runs when config.toml holds the hash of its definition at its key, and not when the definition
// changed since; --dangerously-bypass-hook-trust runs it either way.
func TestAHookRunsWhenItsHashIsTrustedAndNotWhenItChanged(t *testing.T) {
	ran, _ := trustRun(t, scenario{Untrusted: true, HomeFiles: map[string]string{"config.toml": trustedUser}})
	assert.Equal(t, []string{"user"}, ran)

	changed := trustedUser[:len(trustedUser)-len(userHookHash)-3] + "sha256:00\"\n"
	ran, _ = trustRun(t, scenario{Untrusted: true, HomeFiles: map[string]string{"config.toml": changed}})
	assert.Empty(t, ran, "the hash is not the definition's")

	ran, _ = trustRun(t, scenario{Untrusted: true, BypassTrust: true})
	assert.Equal(t, []string{"user"}, ran, "the user layer's hook runs under the bypass; the project's layer is not loaded")
}

// The project layer loads only in a trusted project: one the sandbox of the run trusts (workspace-write,
// danger-full-access or the bypass, not read-only nor none) or config.toml lists. Its hooks must be trusted
// as any, and a run that trusts the project by its sandbox remembers it in config.toml.
// sr:proves hooks-all-matching-run/codex
func TestProjectHooksLoadOnlyInATrustedProject(t *testing.T) {
	// what the real runs left in config.toml (runs/project-hooks-trust-*): a trusted project, or none
	assert.Contains(t, readFile(t, filepath.Join(loadRecording(t, "project-hooks-trust-workspace-write").sample, "config.toml")), "[projects.\"<RUN>\"]\ntrust_level = \"trusted\"")
	assert.NotContains(t, readFile(t, filepath.Join(loadRecording(t, "project-hooks-trust-read-only").sample, "config.toml")), "[projects.")
	assert.NotContains(t, readFile(t, filepath.Join(loadRecording(t, "project-hooks-trust-no-sandbox").sample, "config.toml")), "[projects.")
	both := map[string]string{"config.toml": trustedUser + trustedProject}
	for _, sandbox := range []string{"", "read-only"} {
		ran, got := trustRun(t, scenario{Untrusted: true, Sandbox: sandbox, HomeFiles: both})
		assert.Equal(t, []string{"user"}, ran, "sandbox %q: the project's hooks are not loaded", sandbox)
		cfg, _ := os.ReadFile(filepath.Join(got.Home, "config.toml"))
		assert.NotContains(t, string(cfg), "[projects.", "nothing is remembered")
	}
	for _, sandbox := range []string{"workspace-write", "danger-full-access"} {
		ran, got := trustRun(t, scenario{Untrusted: true, Sandbox: sandbox, HomeFiles: both})
		assert.Equal(t, []string{"project", "user"}, ran, "sandbox %s", sandbox)
		cfg, _ := os.ReadFile(filepath.Join(got.Home, "config.toml"))
		assert.Contains(t, string(cfg), "[projects.\""+got.Repo+"\"]\ntrust_level = \"trusted\"", "the trusted project is remembered")
	}
	ran, _ := trustRun(t, scenario{Untrusted: true, HomeFiles: map[string]string{"config.toml": trustedUser + trustedProject + trustsRepo}})
	assert.Equal(t, []string{"project", "user"}, ran, "config.toml trusts the project")

	// a trusted project does not trust its hooks
	ran, _ = trustRun(t, scenario{Untrusted: true, HomeFiles: map[string]string{"config.toml": trustedUser + trustsRepo}})
	assert.Equal(t, []string{"user"}, ran)
}

// --disable hooks loads no hook, trusted, bypassed or not.
// sr:proves hooks-all-matching-run/codex
func TestDisableHooksLoadsNone(t *testing.T) {
	ran, _ := trustRun(t, scenario{BypassTrust: true, Args: []string{"--disable", "hooks"}})
	assert.Empty(t, ran)
}

// --ignore-user-config leaves config.toml unread: the trust in it counts for nothing; the user layer's
// hooks.json still loads, so a bypassed hook runs.
// sr:proves hooks-all-matching-run/codex
func TestIgnoreUserConfigIgnoresTheTrustInConfigToml(t *testing.T) {
	ran, _ := trustRun(t, scenario{Untrusted: true, Args: []string{"--ignore-user-config"}, HomeFiles: map[string]string{"config.toml": trustedUser}})
	assert.Empty(t, ran)
	ran, _ = trustRun(t, scenario{Untrusted: true, BypassTrust: true, Args: []string{"--ignore-user-config"}})
	assert.Equal(t, []string{"user"}, ran)
}

// -s takes the three modes Codex names and refuses another.
func TestSandboxTakesCodexsThreeModes(t *testing.T) {
	for _, mode := range []string{"read-only", "workspace-write", "danger-full-access"} {
		got := execIn(t, t.TempDir(), "--skip-git-repo-check", "-s", mode, "go")
		assert.Zero(t, got.Code, "%s: %s", mode, got.Stderr)
	}
	got := execIn(t, t.TempDir(), "--skip-git-repo-check", "--sandbox", "bogus", "go")
	assert.NotZero(t, got.Code)
	assert.Contains(t, got.Stderr, "invalid value 'bogus' for '--sandbox <SANDBOX_MODE>'")
}

// A plugin's hook command is told its plugin's root and data directory, under Claude Code's names and its own
// (runs/plugin-hook-env); a user's or project's hook is told none.
// sr:proves plugin-hooks/codex
func TestAPluginHookIsToldItsRootAndDataDirectories(t *testing.T) {
	hook := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"env | grep -E '^(CLAUDE_)?PLUGIN_' | sort >> \"$HOOK_LOG\""}]}]}}`
	got := execMock(t, scenario{
		HomeFiles: map[string]string{"config.toml": declaredMk + p1On},
		Files: map[string]string{
			"mk/.agents/plugins/marketplace.json": pluginMarketplace,
			"mk/plugins/p1/hooks/hooks.json":      hook,
		},
		Script: callThenResult, Prompt: "go", Env: withCalls(t, "true"),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	log, _ := os.ReadFile(filepath.Join(got.Tmp, "hook.log"))
	root, data := got.Home+"/plugins/cache/mk/p1/local", got.Home+"/plugins/data/p1-mk"
	assert.Equal(t, "CLAUDE_PLUGIN_DATA="+data+"\nCLAUDE_PLUGIN_ROOT="+root+"\nPLUGIN_DATA="+data+"\nPLUGIN_ROOT="+root+"\n", string(log))
}
