package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 011_plugin_hooks exercises the END-TO-END plugin hook path the unit tests
// cannot reach: the compiled mock binary reading real settings, resolving a
// marketplace declared in extraKnownMarketplaces, loading the plugin's
// hooks/hooks.json, expanding ${CLAUDE_PLUGIN_ROOT}, and executing the
// plugin-provided hook script.
//
// sr:docs https://code.claude.com/docs/en/plugin-marketplaces

// seedDirectoryMarketplace builds a directory-source marketplace at mpRoot with
// one plugin whose Stop hook appends to logFile. Returns the plugin install dir
// so the test can assert ${CLAUDE_PLUGIN_ROOT} expanded to it.
func seedDirectoryMarketplace(t *testing.T, mpRoot, mpName, plugin, logFile string) string {
	t.Helper()
	// .claude-plugin/marketplace.json — identity verified by the loader.
	mpMeta := filepath.Join(mpRoot, ".claude-plugin")
	require.NoError(t, os.MkdirAll(mpMeta, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(mpMeta, "marketplace.json"),
		[]byte(`{"name":"`+mpName+`"}`), 0o644))

	pluginDir := filepath.Join(mpRoot, "plugins", plugin)
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "hooks"), 0o755))

	// The hook echoes its own dir (resolved from ${CLAUDE_PLUGIN_ROOT}) so the
	// test proves the placeholder expanded to the install dir.
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "hooks", "on_stop.sh"),
		[]byte("#!/bin/sh\ncat >/dev/null\necho \"stop-hook-ran root="+pluginDir+"\" >> \""+logFile+"\"\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "hooks", "hooks.json"), []byte(`{
  "hooks": {
    "Stop": [
      {"matcher":"*","hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/on_stop.sh\""}]}
    ]
  }
}`), 0o644))
	return pluginDir
}

// writeProjectSettings writes .claude/settings.local.json into repoDir.
func writeProjectSettings(t *testing.T, repoDir, body string) {
	t.Helper()
	claudeDir := filepath.Join(repoDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(body), 0o644))
}

// stopScenario writes a scenario that ends a turn (fires Stop).
func stopScenario(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "scenario.sh")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+
		`printf '%s\n' '{"type":"result","subtype":"success","result":"done","is_error":false}'`+"\n"), 0o755))
	return p
}

// TestT011_01_PluginHookFiresViaMarketplace proves the full chain: a plugin
// enabled in settings, its marketplace declared (directory source) in
// extraKnownMarketplaces, has its Stop hook fired with ${CLAUDE_PLUGIN_ROOT}
// expanded to the plugin install dir.
// staged:proves plugin-hooks/claude
func TestT011_01_PluginHookFiresViaMarketplace(t *testing.T) {
	dir := t.TempDir()
	mpRoot := t.TempDir()
	cache := t.TempDir()
	logFile := filepath.Join(dir, "hook-log.txt")

	installDir := seedDirectoryMarketplace(t, mpRoot, "a10n-marketplace",
		"a10n-task-manager-capability", logFile)

	writeProjectSettings(t, dir, `{
  "enabledPlugins": {"a10n-task-manager-capability@a10n-marketplace": true},
  "extraKnownMarketplaces": {
    "a10n-marketplace": {"source": {"source": "directory", "path": "`+mpRoot+`"}}
  }
}`)

	script := stopScenario(t, dir)
	out, code := runInDir(t, dir,
		[]string{"CLAUDE_CODE_PLUGIN_CACHE_DIR=" + cache},
		"--script", script,
		"--plugin-cache-dir", cache,
		"--session-id", "s1",
		"--project-dir", dir,
		"-p", "go",
	)
	require.Equal(t, 0, code, "mock should exit 0; output:\n%s", out)

	logBytes, err := os.ReadFile(logFile)
	require.NoError(t, err, "plugin Stop hook did not fire — plugin hooks not loaded")
	assert.Contains(t, string(logBytes), "stop-hook-ran root="+installDir,
		"hook ran AND ${CLAUDE_PLUGIN_ROOT} expanded to the install dir")
}

// TestT011_02_UndeclaredMarketplaceDoesNotFire is the no-fallback control: the
// plugin is enabled but its marketplace is NOT in extraKnownMarketplaces, so no
// hook resolves.
// staged:proves plugin-hooks/claude
func TestT011_02_UndeclaredMarketplaceDoesNotFire(t *testing.T) {
	dir := t.TempDir()
	mpRoot := t.TempDir()
	cache := t.TempDir()
	logFile := filepath.Join(dir, "hook-log.txt")

	// Marketplace seeded on disk, but NOT declared in settings.
	seedDirectoryMarketplace(t, mpRoot, "a10n-marketplace", "a10n-task-manager-capability", logFile)

	writeProjectSettings(t, dir, `{
  "enabledPlugins": {"a10n-task-manager-capability@a10n-marketplace": true}
}`)

	script := stopScenario(t, dir)
	_, code := runInDir(t, dir,
		[]string{"CLAUDE_CODE_PLUGIN_CACHE_DIR=" + cache},
		"--script", script,
		"--plugin-cache-dir", cache,
		"--session-id", "s1",
		"--project-dir", dir,
		"-p", "go",
	)
	require.Equal(t, 0, code)

	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "undeclared marketplace must not fire any hook (no fallback)")
}

// TestT011_03_DisabledPluginDoesNotFire: marketplace declared, plugin present,
// but enabledPlugins = false → no hook.
// staged:proves plugin-hooks/claude
func TestT011_03_DisabledPluginDoesNotFire(t *testing.T) {
	dir := t.TempDir()
	mpRoot := t.TempDir()
	cache := t.TempDir()
	logFile := filepath.Join(dir, "hook-log.txt")

	seedDirectoryMarketplace(t, mpRoot, "a10n-marketplace", "a10n-task-manager-capability", logFile)

	writeProjectSettings(t, dir, `{
  "enabledPlugins": {"a10n-task-manager-capability@a10n-marketplace": false},
  "extraKnownMarketplaces": {
    "a10n-marketplace": {"source": {"source": "directory", "path": "`+mpRoot+`"}}
  }
}`)

	script := stopScenario(t, dir)
	_, code := runInDir(t, dir,
		[]string{"CLAUDE_CODE_PLUGIN_CACHE_DIR=" + cache},
		"--script", script,
		"--plugin-cache-dir", cache,
		"--session-id", "s1",
		"--project-dir", dir,
		"-p", "go",
	)
	require.Equal(t, 0, code)

	_, err := os.Stat(logFile)
	assert.True(t, os.IsNotExist(err), "disabled plugin must not fire its hook")
}
