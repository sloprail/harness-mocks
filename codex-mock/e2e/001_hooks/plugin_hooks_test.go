package e2e

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The recorded run runs/plugin-hooks: a local marketplace mk lists plugins p1
// (enabled in config.toml) and p2 (disabled), config.toml also enables p3 of a
// marketplace it never declares; every plugin has a PreToolUse hook logging its
// own name, and the user layer's hooks.json has one logging "project". Only p1
// and the project's ran. (The recording set the plugins up with `codex plugin`;
// the mock reads the config.toml and the marketplace directory that wrote.)

const pluginMarketplace = `{"name":"mk","plugins":[
 {"name":"p1","source":{"source":"local","path":"./plugins/p1"}},
 {"name":"p2","source":{"source":"local","path":"./plugins/p2"}}]}`

func pluginHooksJSON(name string) string {
	return `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"\"$(git rev-parse --show-toplevel)\"/hook.sh ` + name + `"}]}]}}`
}

const (
	declaredMk = "[marketplaces.mk]\nsource_type = \"local\"\nsource = \"{REPO}/mk\"\n\n"
	p1On       = "[plugins.\"p1@mk\"]\nenabled = true\n\n"
	p2Off      = "[plugins.\"p2@mk\"]\nenabled = false\n\n"
	p3Orphan   = "[plugins.\"p3@undeclared\"]\nenabled = true\n"
)

// runPlugins replays the recorded scenario with the given config.toml and
// returns which hooks ran.
func runPlugins(t *testing.T, rec recording, configToml string) []string {
	t.Helper()
	files := map[string]string{
		"hook.sh":                             readFile(t, filepath.Join(rec.setup, "hook.sh")),
		"mk/.agents/plugins/marketplace.json": pluginMarketplace,
	}
	for _, p := range []string{"p1", "p2"} {
		files["mk/plugins/"+p+"/.codex-plugin/plugin.json"] = `{"name":"` + p + `","version":"1.0.0"}`
		files["mk/plugins/"+p+"/hooks/hooks.json"] = pluginHooksJSON(p)
	}
	got := execMock(t, scenario{
		HooksJSON: readFile(t, filepath.Join(rec.setup, "hooks.json")),
		HomeFiles: map[string]string{"config.toml": configToml},
		Files:     files,
		Script:    callThenResult,
		Prompt:    strings.TrimSpace(readFile(t, filepath.Join(rec.setup, "prompt.txt"))),
		Env:       withCalls(t, rec.calls...),
	})
	require.Equal(t, 0, got.Code, got.Stderr)
	return ranHooks(got.hookLog())
}

// The hooks of a plugin the config enables, from a marketplace it declares, run
// alongside the user layer's own; a disabled plugin and one of an undeclared
// marketplace contribute none (runs/plugin-hooks).
// sr:proves plugin-hooks/codex
func TestPluginHooksRunAlongsideTheProjectsOwn(t *testing.T) {
	rec := loadRecording(t, "plugin-hooks")
	want := ranHooks(jsonLines(readFile(t, filepath.Join(rec.sample, "payloads.jsonl"))))
	require.Equal(t, []string{"p1", "project"}, want, "what the recording shows")

	assert.Equal(t, want, runPlugins(t, rec, declaredMk+p1On+p2Off+p3Orphan))

	// the marketplace not declared: its plugin contributes nothing, though enabled
	assert.Equal(t, []string{"project"}, runPlugins(t, rec, p1On+p2Off+p3Orphan))
	// the plugin not enabled: nothing either, though its marketplace is declared
	assert.Equal(t, []string{"project"}, runPlugins(t, rec, declaredMk+strings.Replace(p1On, "true", "false", 1)+p2Off))
}

// ranHooks are the names the hooks logged, sorted.
func ranHooks(lines []map[string]any) []string {
	var out []string
	for _, l := range lines {
		out = append(out, l["hook_ran"].(string))
	}
	sort.Strings(out)
	return out
}
