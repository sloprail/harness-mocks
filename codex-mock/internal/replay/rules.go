package replay

import (
	"encoding/json"
	"regexp"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// Rules are what a recording and a replay of it may differ in, and why:
// what is dropped is no capability cell's; what differs per run but is there in every run
// keeps its key and loses its value (MaskKeys), so a payload without it still differs.
func Rules(repo, root string) rp.Rules {
	re := regexp.MustCompile
	return rp.Rules{
		// what differs in every run but is there in every run: the key is compared, its value is not
		MaskKeys: []string{
			"usage",                  // token counts: the mock has no model
			"model",                  // gpt-5.6-luna against the mock's name
			"transcript_path",        // where the rollout is kept: a path with the time and the thread id in it
			"agent_transcript_path",  // a sub-agent's
			"turn_id", "tool_use_id", // ids that differ in every run
			"wall_time_seconds", "duration_ms", // timings
		},
		DropKeys: []string{
			"script", // the mock's own spawn_agent parameter: the sub-agent's script
		},
		Rewrite: map[string]func(string) string{
			// a `ps` listing is the host's: only the job's own processes are the behaviour
			"aggregated_output": scratchText,
			"tool_response":     func(s string) string { return withoutNickname(scratchText(s)) },
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

// withoutNickname is a spawn_agent receipt ({"agent_id", "nickname"}) with the
// nickname, which Codex picks at random, as <NICKNAME>; any other text is returned as it is.
func withoutNickname(text string) string {
	var r map[string]any
	if json.Unmarshal([]byte(text), &r) != nil || r["agent_id"] == nil || r["nickname"] == nil {
		return text
	}
	r["nickname"] = "<NICKNAME>"
	b, _ := json.Marshal(r)
	return string(b)
}
