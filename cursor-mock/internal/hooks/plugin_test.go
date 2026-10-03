package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePlugin(t *testing.T, dir, manifest, hooksRel string) {
	t.Helper()
	hookFile := filepath.Join(dir, hooksRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(hookFile), 0o755))
	require.NoError(t, os.WriteFile(hookFile, []byte(`{"version":1,"hooks":{"beforeShellExecution":[{"command":"plugin-hook"}]}}`), 0o644))
	if manifest != "" {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".cursor-plugin"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".cursor-plugin", "plugin.json"), []byte(manifest), 0o644))
	}
}

func commands(c Config) (out []string) {
	for _, e := range c.Entries("beforeShellExecution") {
		out = append(out, e.Command)
	}
	return out
}

// A plugin loaded from a directory contributes its hooks after the project's:
// from hooks/hooks.json when its manifest names no hooks file (the doc's
// default location), from the file the manifest names when it does, and
// nothing when the directory is not loaded.
// sr:proves plugin-hooks/cursor
func TestPluginHooksAreFoundByDefaultOrByTheManifest(t *testing.T) {
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".cursor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".cursor", "hooks.json"),
		[]byte(`{"version":1,"hooks":{"beforeShellExecution":[{"command":"project-hook"}]}}`), 0o644))

	byDefault, named, unloaded := filepath.Join(ws, "plugins", "a"), filepath.Join(ws, "plugins", "b"), filepath.Join(ws, "plugins", "c")
	writePlugin(t, byDefault, `{"name":"a"}`, "hooks/hooks.json")
	writePlugin(t, named, `{"name":"b","hooks":"./config/my-hooks.json"}`, "config/my-hooks.json")
	writePlugin(t, unloaded, `{"name":"c"}`, "hooks/hooks.json")

	c, err := Load(ws, "plugins/a", named)
	require.NoError(t, err)
	assert.Equal(t, []string{"project-hook", "plugin-hook", "plugin-hook"}, commands(c), "project's first, then each loaded plugin's; c is not loaded")

	c, err = Load(ws)
	require.NoError(t, err)
	assert.Equal(t, []string{"project-hook"}, commands(c))
}
