package replay

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A recorded run's options the mock can be given are given; a token limit is dropped (the script
// compacts where the harness did); an option the adapter cannot give is an error, never ignored.
func TestRunFlagsGivesWhatTheMockTakesAndRefusesTheRest(t *testing.T) {
	pass, err := runFlags("-c\nagents.max_depth=3\n-C\n.codex\n--ephemeral\n")
	require.NoError(t, err)
	assert.Equal(t, []string{"-c", "agents.max_depth=3", "-C", ".codex", "--ephemeral"}, pass)

	pass, err = runFlags("-c\nmodel_auto_compact_token_limit=4000\n")
	require.NoError(t, err)
	assert.Empty(t, pass, "the token limit is the script's compactions, not a flag")

	for _, bad := range []string{"--output-schema\nschema.json", "-c\nsomething=1", "-C"} {
		_, err := runFlags(bad)
		assert.Error(t, err, bad)
	}
}

// A resume of a session by its id is given to the mock as it was recorded.
func TestRunFlagsGivesAResumeByItsID(t *testing.T) {
	pass, err := runFlags("resume\n00000000-0000-4000-8000-0000000000ff\n")
	require.NoError(t, err)
	assert.Equal(t, []string{"resume", "00000000-0000-4000-8000-0000000000ff"}, pass)
}

// A recorded run's env file is given to the mock's process and its prepare.sh is run before the mock, with
// the stubs: `codex plugin` writes the config.toml the mock reads, and `sed -i ”` works on any sed.
func TestPrepareRunsTheRecordedScriptWithTheCodexAndSedStubs(t *testing.T) {
	dir := func() string { d, _ := filepath.EvalSymlinks(t.TempDir()); return d } // as the script's $PWD is
	root, repo, home := dir(), dir(), dir()
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "mk", ".agents", "plugins"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "mk", ".agents", "plugins", "marketplace.json"), []byte(`{"name":"mk"}`), 0o644))
	script := "set -e\ncodex plugin marketplace add \"$PWD/mk\"\ncodex plugin add p1@mk\nsed -i '' 's/true/false/' \"$CODEX_HOME/config.toml\"\n"
	require.NoError(t, prepare(context.Background(), script, root, repo, home, []string{"CODEX_HOME=" + home, "PATH=" + os.Getenv("PATH")}))
	b, err := os.ReadFile(filepath.Join(home, "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "[marketplaces.mk]\nsource_type = \"local\"\nsource = \""+filepath.Join(repo, "mk")+"\"")
	assert.Contains(t, string(b), "[plugins.\"p1@mk\"]\nenabled = false", "the sed rewrote it")

	err = prepare(context.Background(), "codex exec x\n", root, repo, home, []string{"CODEX_HOME=" + home, "PATH=" + os.Getenv("PATH")})
	assert.Error(t, err, "another codex command is an error")
}

// A run made without --json prints only the agent's last message: its lines are the compared stream,
// as raw lines; with --json they are events.
func TestWithoutJSONTheStreamIsTheRawLines(t *testing.T) {
	text, err := parseStream("DONE\n", []string{"--skip-git-repo-check"})
	require.NoError(t, err)
	assert.Equal(t, []map[string]any{{"raw": "DONE"}}, text)
	events, err := parseStream("{\"type\":\"turn.started\"}\n", []string{"--json"})
	require.NoError(t, err)
	assert.Equal(t, "turn.started", events[0]["type"])
}
