package e2e

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The mock fails fast on what it does not implement (adr/fail-fast-unimplemented):
// a flag it implements nothing of, or a -c key it does not read, is refused with a
// clear error and nothing runs, so a recording made with it cannot be replayed and
// pass as a run without it.
// sr:proves noninteractive-run/codex
func TestUnimplementedFlagsAreRefused(t *testing.T) {
	for _, args := range [][]string{
		{"--enable", "multi_agent_v2"}, {"--disable", "multi_agent_v2"},
		{"--output-schema", "schema.json"}, {"-o", "last.txt"},
		{"--profile", "p"}, {"--color", "never"},
		{"--ignore-rules"}, {"--strict-config"}, {"--approve-for-me"}, {"--thread-source", "x"},
		{"-c", "model_reasoning_effort=high"},
	} {
		t.Run(args[0], func(t *testing.T) {
			r := execIn(t, t.TempDir(), append([]string{"--skip-git-repo-check"}, append(args, "go")...)...)
			assert.NotZero(t, r.Code)
			assert.Contains(t, r.Stderr, "not implemented by the mock")
			assert.Empty(t, r.hookLog(), "nothing ran")
		})
	}
}

// --enable and --disable are implemented for the hooks feature only: any other feature is refused by name,
// and so is naming hooks both ways in one run.
// sr:proves noninteractive-run/codex
func TestOnlyTheHooksFeatureCanBeSwitched(t *testing.T) {
	for _, args := range [][]string{{"--enable", "multi_agent_v2"}, {"--disable", "unified_exec"}, {"--enable", "hooks", "--disable", "hooks"}} {
		r := execIn(t, t.TempDir(), append([]string{"--skip-git-repo-check"}, append(args, "go")...)...)
		assert.NotZero(t, r.Code, args)
		assert.Contains(t, r.Stderr, "not implemented by the mock", args)
		assert.Empty(t, r.hookLog(), "nothing ran")
	}
}

// --full-auto, the deprecated compatibility flag the docs name, is a flag the mock does not know at
// all: refused as unknown, not accepted.
func TestFullAutoIsRefused(t *testing.T) {
	r := execIn(t, t.TempDir(), "--skip-git-repo-check", "--full-auto", "go")
	assert.NotZero(t, r.Code)
	assert.Contains(t, r.Stderr, "unknown flag: --full-auto")
	assert.Empty(t, r.hookLog(), "nothing ran")
}

// `codex exec resume --last` resumes the newest session; the mock resumes only a session named by its id,
// so --last is a flag it does not know: refused, nothing runs.
func TestResumeLastIsRefused(t *testing.T) {
	r := execIn(t, t.TempDir(), "--skip-git-repo-check", "resume", "--last", "go")
	assert.NotZero(t, r.Code)
	assert.Contains(t, r.Stderr, "unknown flag: --last")
	assert.Empty(t, r.hookLog(), "nothing ran")
}
