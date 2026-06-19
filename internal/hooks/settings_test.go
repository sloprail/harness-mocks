package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeClaudeSettings writes .claude/<name> under repoDir.
func writeClaudeSettings(t *testing.T, repoDir, name, body string) {
	t.Helper()
	dir := filepath.Join(repoDir, ".claude")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
}

// LoadSettings resolves plugin hooks from enabledPlugins + a directory-source
// marketplace declared in extraKnownMarketplaces, expanding ${CLAUDE_PLUGIN_ROOT}.
func TestLoadSettings_PluginHooksFromDirectoryMarketplace(t *testing.T) {
	repo := t.TempDir()
	mpRoot := t.TempDir()
	writeMarketplace(t, mpRoot, "a10n-marketplace")
	pluginDir := seedPluginHooks(t, mpRoot, "a10n-task-manager-capability", `{
  "hooks": {
    "PreToolUse": [
      {"matcher":"Agent","hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/pre_tool_use.sh\""}]}
    ]
  }
}`)

	writeClaudeSettings(t, repo, "settings.local.json", `{
  "enabledPlugins": {"a10n-task-manager-capability@a10n-marketplace": true},
  "extraKnownMarketplaces": {
    "a10n-marketplace": {"source": {"source": "directory", "path": "`+mpRoot+`"}}
  }
}`)

	got, err := LoadSettings(repo, t.TempDir())
	require.NoError(t, err)

	pre := got.EntriesFor(EventPreToolUse, "Agent")
	require.Len(t, pre, 1)
	assert.Equal(t, filepath.Join(pluginDir, "hooks", "pre_tool_use.sh"), trimQuotes(pre[0].Command))
}

// Inline settings.json hooks and plugin-provided hooks coexist for the same event.
func TestLoadSettings_InlineAndPluginHooksMerge(t *testing.T) {
	repo := t.TempDir()
	mpRoot := t.TempDir()
	writeMarketplace(t, mpRoot, "mp")
	seedPluginHooks(t, mpRoot, "p", `{"hooks":{"Stop":[{"matcher":"*","hooks":[{"type":"command","command":"PLUGIN"}]}]}}`)

	writeClaudeSettings(t, repo, "settings.json",
		`{"hooks":{"Stop":[{"matcher":"*","hooks":[{"type":"command","command":"INLINE"}]}]}}`)
	writeClaudeSettings(t, repo, "settings.local.json", `{
  "enabledPlugins": {"p@mp": true},
  "extraKnownMarketplaces": {"mp": {"source": {"source": "directory", "path": "`+mpRoot+`"}}}
}`)

	got, err := LoadSettings(repo, t.TempDir())
	require.NoError(t, err)

	stop := got.EntriesFor(EventStop, "")
	require.Len(t, stop, 2)
	cmds := []string{stop[0].Command, stop[1].Command}
	assert.Contains(t, cmds, "INLINE")
	assert.Contains(t, cmds, "PLUGIN")
}

// With no enabledPlugins, LoadSettings returns only inline hooks (back-compat).
func TestLoadSettings_NoPluginsIsInlineOnly(t *testing.T) {
	repo := t.TempDir()
	writeClaudeSettings(t, repo, "settings.json",
		`{"hooks":{"PreToolUse":[{"matcher":"Agent","hooks":[{"type":"command","command":"INLINE"}]}]}}`)

	got, err := LoadSettings(repo, t.TempDir())
	require.NoError(t, err)

	pre := got.EntriesFor(EventPreToolUse, "Agent")
	require.Len(t, pre, 1)
	assert.Equal(t, "INLINE", pre[0].Command)
}

// An enabled plugin whose marketplace is NOT declared in extraKnownMarketplaces
// resolves to nothing — there is no fallback to "name-after-@ as a path".
func TestLoadSettings_UndeclaredMarketplaceNoFallback(t *testing.T) {
	repo := t.TempDir()
	writeClaudeSettings(t, repo, "settings.local.json",
		`{"enabledPlugins":{"p@ghost-marketplace":true}}`)

	got, err := LoadSettings(repo, t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, got.Hooks)
}

// extraKnownMarketplaces declared in settings.json is honoured for plugins
// enabled in settings.local.json (fields merge across both files).
func TestLoadSettings_MarketplaceAndEnableAcrossFiles(t *testing.T) {
	repo := t.TempDir()
	mpRoot := t.TempDir()
	writeMarketplace(t, mpRoot, "a10n-marketplace")
	seedPluginHooks(t, mpRoot, "p", `{"hooks":{"SessionStart":[{"matcher":"*","hooks":[{"type":"command","command":"A"}]}]}}`)

	// Marketplace declared in project settings.json...
	writeClaudeSettings(t, repo, "settings.json", `{
  "extraKnownMarketplaces": {"a10n-marketplace": {"source": {"source": "directory", "path": "`+mpRoot+`"}}}
}`)
	// ...plugin enabled in local settings.
	writeClaudeSettings(t, repo, "settings.local.json", `{"enabledPlugins":{"p@a10n-marketplace":true}}`)

	got, err := LoadSettings(repo, t.TempDir())
	require.NoError(t, err)

	start := got.EntriesFor(EventSessionStart, "")
	require.Len(t, start, 1)
	assert.Equal(t, "A", start[0].Command)
}
