package e2e

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/plugin-install: an enabled plugin is installed into the cache of the run's
// CODEX_HOME, plugins/cache/<marketplace>/<plugin>/<version> (the manifest's version, "local" when it names
// none): the whole directory, dotfiles too, but not a symbolic link; a new version replaces the old whole.
// The plugin's hooks run from there, and its hook command is told that path as PLUGIN_ROOT.

func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		rel, _ := filepath.Rel(dir, p)
		out = append(out, rel)
		return err
	}))
	sort.Strings(out)
	return out
}

func installScenario(version string) scenario {
	manifest := `{"name":"p1"}`
	if version != "" {
		manifest = `{"name":"p1","version":"` + version + `"}`
	}
	return scenario{
		HomeFiles: map[string]string{"config.toml": declaredMk + p1On},
		Files: map[string]string{
			"mk/.agents/plugins/marketplace.json":     pluginMarketplace,
			"mk/plugins/p1/.codex-plugin/plugin.json": manifest,
			"mk/plugins/p1/hooks/hooks.json":          pluginHooksJSON("p1"),
			"mk/plugins/p1/.hidden/h":                 "h",
			"mk/plugins/p1/scripts/run.sh":            "r",
		},
		Script: callThenResult, Prompt: "go",
	}
}

// sr:proves plugin-hooks/codex
func TestAPluginIsInstalledIntoTheCacheOfTheRunsHome(t *testing.T) {
	rec := loadRecording(t, "plugin-install")
	want := readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))
	require.Contains(t, want, `"root":"<TMP>/home/.codex/plugins/cache/mk/p1/1.1.0"`)
	require.Contains(t, want, `plugins/cache/mk/p2/local"`)

	s := installScenario("1.0.0")
	s.Files["hook.sh"] = "#!/bin/sh\ncat >/dev/null\nexit 0\n"
	s.Env = withCalls(t, "true")
	got := execMock(t, s)
	require.Equal(t, 0, got.Code, got.Stderr)
	cache := filepath.Join(got.Home, "plugins", "cache", "mk", "p1", "1.0.0")
	assert.Equal(t, []string{".", ".codex-plugin", ".codex-plugin/plugin.json", ".hidden", ".hidden/h", "hooks", "hooks/hooks.json", "scripts", "scripts/run.sh"}, filesUnder(t, cache))

	// a manifest with no version installs as "local"
	s = installScenario("")
	s.Env = withCalls(t, "true")
	got = execMock(t, s)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.DirExists(t, filepath.Join(got.Home, "plugins", "cache", "mk", "p1", "local"))
}

// A newer version of a plugin replaces the installed one whole: the old version's directory is gone.
func TestANewPluginVersionReplacesTheCachedOne(t *testing.T) {
	first := execMock(t, func() scenario { s := installScenario("1.0.0"); s.Env = withCalls(t, "true"); return s }())
	require.Equal(t, 0, first.Code, first.Stderr)
	assert.DirExists(t, filepath.Join(first.Home, "plugins", "cache", "mk", "p1", "1.0.0"))

	// the marketplace now holds 1.1.0: the same home installs it, and 1.0.0 goes
	require.NoError(t, os.WriteFile(filepath.Join(first.Repo, "mk/plugins/p1/.codex-plugin/plugin.json"), []byte(`{"name":"p1","version":"1.1.0"}`), 0o644))
	again := resumeIn(t, first, first.Repo, threadIDs(first)[0], "again")
	require.Equal(t, 0, again.Code, again.Stderr)
	assert.DirExists(t, filepath.Join(first.Home, "plugins", "cache", "mk", "p1", "1.1.0"))
	assert.NoDirExists(t, filepath.Join(first.Home, "plugins", "cache", "mk", "p1", "1.0.0"))
}

// A run installs under its own CODEX_HOME and leaves the home of the user running it untouched; with no
// CODEX_HOME it uses $HOME/.codex, as codex does.
func TestAPluginInstallLeavesTheRealHomeAlone(t *testing.T) {
	userHome := t.TempDir()
	s := installScenario("1.0.0")
	s.Env = append(withCalls(t, "true"), "HOME="+userHome)
	got := execMock(t, s)
	require.Equal(t, 0, got.Code, got.Stderr)
	assert.DirExists(t, filepath.Join(got.Home, "plugins", "cache", "mk", "p1", "1.0.0"))
	entries, _ := os.ReadDir(userHome)
	assert.Empty(t, entries, "nothing was written under HOME: %v", entries)
	assert.False(t, strings.HasPrefix(got.Home, userHome))
}
