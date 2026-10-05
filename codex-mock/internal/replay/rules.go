package replay

import (
	"regexp"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// Rules are what a recording and a replay of it may differ in, and why:
// no capability cell is about any of it.
// sr:invariant replay-fidelity
func Rules(repo, root string) rp.Rules {
	re := regexp.MustCompile
	return rp.Rules{
		DropKeys: []string{
			"usage",                  // token counts: the mock has no model
			"model",                  // gpt-5.6-luna against the mock's name
			"transcript_path",        // where the rollout is kept: a path
			"cwd",                    // the run's directory: a path
			"turn_id", "tool_use_id", // ids that differ in every run
			"wall_time_seconds", "duration_ms", // timings
			"agent_transcript_path",
			"script", // the mock's own spawn_agent parameter: the sub-agent's script
		},
		Rewrite: map[string]func(string) string{
			// a `ps` listing is the host's: only the job's own processes are the behaviour
			"aggregated_output": scratchText,
			"tool_response":     scratchText,
			// where the harness's own npm package is installed is the machine's
			"CODEX_MANAGED_PACKAGE_ROOT": func(string) string { return "<PKG_ROOT>" },
			// how the shell was invoked is the machine's: the command is what the model asked for
			"command": shellInner,
		},
		Scrub: []rp.Scrub{
			{Re: re(regexp.QuoteMeta(repo)), With: "<RUN>"},
			{Re: re(regexp.QuoteMeta(root)), With: "<TMP>"}, // the run's scratch directory: the repository, CODEX_HOME and TMPDIR sit in it
		},
		IDs: []*regexp.Regexp{re(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)}, // thread and session ids
	}
}
