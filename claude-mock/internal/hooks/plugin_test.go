package hooks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginCacheDir resolution order: explicit override > CLAUDE_CODE_PLUGIN_CACHE_DIR
// env > fixed /tmp/a10n-mock-plugins default.
func TestPluginCacheDir(t *testing.T) {
	t.Run("explicit override wins over env", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_PLUGIN_CACHE_DIR", "/from/env")
		assert.Equal(t, "/explicit", pluginCacheDir("/explicit"))
	})
	t.Run("env used when no explicit override", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_PLUGIN_CACHE_DIR", "/from/env")
		assert.Equal(t, "/from/env", pluginCacheDir(""))
	})
	t.Run("default when neither set", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_PLUGIN_CACHE_DIR", "")
		assert.Equal(t, "/tmp/a10n-mock-plugins", pluginCacheDir(""))
	})
}

// splitPluginKey splits "<name>@<marketplace>" on the LAST '@' and errors when
// there is no '@'.
func TestSplitPluginKey(t *testing.T) {
	t.Run("plain key", func(t *testing.T) {
		name, mp, err := splitPluginKey("a10n-task-manager-capability@a10n-marketplace")
		require.NoError(t, err)
		assert.Equal(t, "a10n-task-manager-capability", name)
		assert.Equal(t, "a10n-marketplace", mp)
	})
	t.Run("splits on last @", func(t *testing.T) {
		name, mp, err := splitPluginKey("scope@x@mp")
		require.NoError(t, err)
		assert.Equal(t, "scope@x", name)
		assert.Equal(t, "mp", mp)
	})
	t.Run("missing @ is an error", func(t *testing.T) {
		_, _, err := splitPluginKey("no-at-sign")
		require.Error(t, err)
	})
}

// groupEnabledByMarketplace keeps only enabled entries and groups plugin names
// under their marketplace.
func TestGroupEnabledByMarketplace(t *testing.T) {
	got := groupEnabledByMarketplace(map[string]bool{
		"a@mp1":      true,
		"b@mp1":      true,
		"c@mp2":      true,
		"disabled@x": false,
	})
	assert.ElementsMatch(t, []string{"a", "b"}, got["mp1"])
	assert.Equal(t, []string{"c"}, got["mp2"])
	_, hasDisabled := got["x"]
	assert.False(t, hasDisabled, "disabled plugin's marketplace must not appear")
}

// marketplaceSlug derives a filesystem-safe cache dir name from a git URL.
func TestMarketplaceSlug(t *testing.T) {
	assert.Equal(t, "a10n-marketplace", marketplaceSlug("git@github.com:a10n-build/a10n-marketplace.git"))
	assert.Equal(t, "a10n-marketplace", marketplaceSlug("https://github.com/a10n-build/a10n-marketplace.git"))
	assert.Equal(t, "a10n-marketplace", marketplaceSlug("https://github.com/a10n-build/a10n-marketplace"))
}

// initGitRepo turns dir into a committed git repo so it looks "cloned".
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".keep"), []byte("x"), 0o644))
	run("init", "-q")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
}

// writeMarketplace writes a .claude-plugin/marketplace.json with the given name
// under root.
func writeMarketplace(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, ".claude-plugin")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marketplace.json"),
		[]byte(`{"name":"`+name+`"}`), 0o644))
}

// writeMarketplaceWithPlugins writes a .claude-plugin/marketplace.json declaring the given
// name plus a plugins[] entry (name, source) per pair in nameSources — the general manifest
// shape a real marketplace uses (unlike writeMarketplace's bare {"name":...} fixture, which
// only exercises the plugins/<name> FALLBACK path).
func writeMarketplaceWithPlugins(t *testing.T, root, name string, nameSources map[string]string) {
	t.Helper()
	dir := filepath.Join(root, ".claude-plugin")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	var plugins []marketplaceManifestItem
	for n, src := range nameSources {
		plugins = append(plugins, marketplaceManifestItem{Name: n, Source: src})
	}
	data, err := json.Marshal(marketplaceManifest{Name: name, Plugins: plugins})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marketplace.json"), data, 0o644))
}

// resolveMarketplace returns the directory-source root whose marketplace.json
// name matches, and "" when no declared marketplace matches.
func TestResolveMarketplace_DirectorySource(t *testing.T) {
	mpRoot := t.TempDir()
	writeMarketplace(t, mpRoot, "a10n-marketplace")

	cfgs := map[string]marketplaceCfg{
		"whatever-key": {Source: marketplaceSource{Source: "directory", Path: mpRoot}},
	}

	t.Run("matches by marketplace.json name, not settings key", func(t *testing.T) {
		got, _, err := resolveMarketplace("a10n-marketplace", cfgs, t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, mpRoot, got)
	})

	t.Run("no declared marketplace matches → empty", func(t *testing.T) {
		got, _, err := resolveMarketplace("not-declared", cfgs, t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, "", got)
	})
}

// ensureCloned reuses an existing clone without touching the network (no remote
// is configured, so any pull/clone attempt would fail).
func TestEnsureCloned_ReusesExistingClone(t *testing.T) {
	cache := t.TempDir()
	clone := filepath.Join(cache, "a10n-marketplace")
	require.NoError(t, os.MkdirAll(clone, 0o755))
	initGitRepo(t, clone)

	// No remote, no network: a reuse must succeed; a re-clone would fail.
	require.NoError(t, ensureCloned("file:///nonexistent-remote", clone))
}

// expandPluginRoot replaces ${CLAUDE_PLUGIN_ROOT} and preserves other ${VAR}.
func TestExpandPluginRoot(t *testing.T) {
	entries := []HookEntry{{
		Matcher: "Agent",
		Hooks: []HandlerSpec{
			{Type: "command", Command: `"${CLAUDE_PLUGIN_ROOT}/hooks/pre.sh"`},
			{Type: "command", Command: `"${CLAUDE_PLUGIN_ROOT}/hooks/x.sh" --bin "${A10N_TASK_MANAGER_BIN}"`},
		},
	}}
	out := expandPluginRoot(entries, "/install/dir")

	assert.Equal(t, `"/install/dir/hooks/pre.sh"`, out[0].Hooks[0].Command)
	assert.Equal(t, `"/install/dir/hooks/x.sh" --bin "${A10N_TASK_MANAGER_BIN}"`, out[0].Hooks[1].Command)
	assert.Equal(t, "Agent", out[0].Matcher)
	// Input not mutated in place.
	assert.Equal(t, `"${CLAUDE_PLUGIN_ROOT}/hooks/pre.sh"`, entries[0].Hooks[0].Command)
}

// seedPluginHooks writes <root>/plugins/<plugin>/hooks/hooks.json.
func seedPluginHooks(t *testing.T, root, plugin, hooksJSON string) string {
	t.Helper()
	dir := filepath.Join(root, "plugins", plugin)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hooks", "hooks.json"), []byte(hooksJSON), 0o644))
	return dir
}

// loadPluginHooks resolves a directory-source marketplace, reads each enabled
// plugin's hooks.json, and appends the expanded hooks to dst.
// staged:proves plugin-hooks/claude
func TestLoadPluginHooks_DirectoryMarketplace(t *testing.T) {
	mpRoot := t.TempDir()
	writeMarketplace(t, mpRoot, "a10n-marketplace")
	pluginDir := seedPluginHooks(t, mpRoot, "a10n-task-manager-capability", `{
  "hooks": {
    "SubagentStart": [
      {"matcher":"*","hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/subagent_start.sh\""}]}
    ]
  }
}`)

	dst := &Settings{Hooks: map[EventName][]HookEntry{}}
	loadPluginHooks(dst,
		t.TempDir(),
		map[string]bool{"a10n-task-manager-capability@a10n-marketplace": true},
		map[string]marketplaceCfg{"k": {Source: marketplaceSource{Source: "directory", Path: mpRoot}}},
	)

	got := dst.EntriesFor(EventSubagentStart, "")
	require.Len(t, got, 1)
	assert.Equal(t, filepath.Join(pluginDir, "hooks", "subagent_start.sh"), trimQuotes(got[0].Command))
}

// A plugin whose marketplace is NOT declared in extraKnownMarketplaces does not
// resolve — there is no fallback.
// staged:proves plugin-hooks/claude
func TestLoadPluginHooks_NoFallbackWhenMarketplaceUndeclared(t *testing.T) {
	dst := &Settings{Hooks: map[EventName][]HookEntry{}}
	loadPluginHooks(dst,
		t.TempDir(),
		map[string]bool{"some-plugin@ghost-marketplace": true},
		map[string]marketplaceCfg{}, // nothing declared
	)
	assert.Empty(t, dst.Hooks, "undeclared marketplace must not resolve any hooks")
}

// A plugin whose manifest declares a NON-STANDARD source path (e.g. an in-monorepo
// marketplace whose manifest sits at the repo root but whose plugins live under a
// subdirectory) resolves via that declared path, not the plugins/<name> fallback — the
// real regression this covers: a repo root .claude-plugin/marketplace.json with
// "source": "./marketplace/plugins/foo" must find the plugin under marketplace/plugins/foo,
// not <root>/plugins/foo (which does not exist in that layout).
// staged:proves plugin-hooks/claude
func TestLoadPluginHooks_HonorsManifestDeclaredSourcePath(t *testing.T) {
	mpRoot := t.TempDir()
	writeMarketplaceWithPlugins(t, mpRoot, "a10n-marketplace", map[string]string{
		"a10n-spec-capability": "./marketplace/plugins/a10n-spec-capability",
	})
	pluginDir := filepath.Join(mpRoot, "marketplace", "plugins", "a10n-spec-capability")
	require.NoError(t, os.MkdirAll(filepath.Join(pluginDir, "hooks"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "hooks", "hooks.json"), []byte(`{
  "hooks": {
    "Stop": [
      {"matcher":"*","hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/stop.sh\""}]}
    ]
  }
}`), 0o644))
	// The historical plugins/<name> layout must NOT exist — proves resolution used the
	// manifest's declared source, not the fallback (which would 404 here anyway, but an
	// accidental fallback silently returning empty hooks would also make this test pass
	// for the wrong reason without this guard).
	_, statErr := os.Stat(filepath.Join(mpRoot, "plugins", "a10n-spec-capability"))
	require.True(t, os.IsNotExist(statErr), "test setup: plugins/<name> fallback path must not exist")

	dst := &Settings{Hooks: map[EventName][]HookEntry{}}
	loadPluginHooks(dst,
		t.TempDir(),
		map[string]bool{"a10n-spec-capability@a10n-marketplace": true},
		map[string]marketplaceCfg{"k": {Source: marketplaceSource{Source: "directory", Path: mpRoot}}},
	)

	got := dst.EntriesFor(EventStop, "")
	require.Len(t, got, 1)
	assert.Equal(t, filepath.Join(pluginDir, "hooks", "stop.sh"), trimQuotes(got[0].Command))
}

// A disabled plugin contributes nothing even when its marketplace is declared.
// staged:proves plugin-hooks/claude
func TestLoadPluginHooks_DisabledPluginSkipped(t *testing.T) {
	mpRoot := t.TempDir()
	writeMarketplace(t, mpRoot, "a10n-marketplace")
	seedPluginHooks(t, mpRoot, "p", `{"hooks":{"Stop":[{"matcher":"*","hooks":[{"type":"command","command":"x"}]}]}}`)

	dst := &Settings{Hooks: map[EventName][]HookEntry{}}
	loadPluginHooks(dst, t.TempDir(),
		map[string]bool{"p@a10n-marketplace": false},
		map[string]marketplaceCfg{"k": {Source: marketplaceSource{Source: "directory", Path: mpRoot}}},
	)
	assert.Empty(t, dst.Hooks)
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
