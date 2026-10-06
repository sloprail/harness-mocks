package replay

import (
	"regexp"
	"strings"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// Rules are what a recording and a replay of it may differ in, and why: no
// capability cell is about any of it. repo and work are the replay's own paths,
// which the recording has as the capture wrote them: <RUN>, <TMP>, and the run
// directory as claude encodes it into a folder name, <RUN_DIRNAME>. taskIDs
// are the run's task and agent ids (see RunIDs), which have no pattern.
func Rules(repo, work string, taskIDs []string) rp.Rules {
	re := regexp.MustCompile
	enc := re(`[^A-Za-z0-9]`).ReplaceAllString(repo, "-")
	ids := []*regexp.Regexp{
		re(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), // session, hook and prompt ids
		re(`toolu_[0-9A-Za-z]+`), // tool call ids
	}
	if len(taskIDs) > 0 {
		quoted := make([]string, len(taskIDs))
		for i, id := range taskIDs {
			quoted[i] = regexp.QuoteMeta(id)
		}
		ids = append(ids, re(strings.Join(quoted, "|"))) // task and agent ids
	}
	return rp.Rules{
		DropKeys: []string{
			"uuid", "request_id", // ids that differ in every run
			"usage", "modelUsage", "total_cost_usd", "duration_ms", "duration_api_ms", // the model's cost: the mock has no model
			"signature",                                                                                                                 // the model's thinking, signed
			"first_content_frame_ms", "ttft_ms", "ttft_stream_ms", "time_to_request_ms", "fast_mode_state", "fast_mode_disabled_reason", // the real service's latency and mode
		},
		// a sub-agent's spend and the model id its alias resolved to: there in both, their values the run's own
		// and when a task ended, and a resumed start's measures of the conversation (its tokens, its cost, how long ago it ended)
		MaskKeys: []string{"end_time", "scheduledFor", "pre_tokens", "post_tokens", "cumulative_dropped_tokens", "totalTokens", "totalDurationMs", "resolvedModel", "context_tokens", "seconds_since_last_response", "estimated_cache_write_usd",
			// the harness's pid and its messaging secret, as a child's environment names them (a hook's payload): the run's own
			"CLAUDE_PID", "CLAUDE_CODE_MESSAGING_TOKEN",
			// how long a tool took (Glob's durationMs): measured, there in both
			"durationMs",
			// the hash of a sub-agent's report, which names the run's own ids and paths where the replay's words
			// name the recording's
			"harnessSectionHash"},
		// when a frame was written differs in every run; that it has one does not
		Rewrite: map[string]func(string) string{"timestamp": func(string) string { return "<TIME>" }},
		Scrub: []rp.Scrub{
			// a sub-agent trailer's usage line is compared; its counts are the model's spend
			{Re: re(`subagent_tokens: \d+`), With: "subagent_tokens: <MASKED>"},
			{Re: re(`duration_ms: \d+`), With: "duration_ms: <MASKED>"},
			// when a scheduled wakeup falls: the wall clock and the seconds to the next minute's boundary
			{Re: re(`scheduled for \d\d:\d\d:\d\d \(in \d+s\)`), With: "scheduled for <TIME> (in <N>s)"},
			// the same in a task notification's <usage> element
			{Re: re(`<subagent_tokens>\d+</subagent_tokens>`), With: "<subagent_tokens><MASKED></subagent_tokens>"},
			{Re: re(`<duration_ms>\d+</duration_ms>`), With: "<duration_ms><MASKED></duration_ms>"},
			// the same, as a command prints its environment: the pid, the socket named for it, the secret (the capture redacts it), the executable's path
			{Re: re(`CLAUDE_PID=\d+`), With: "CLAUDE_PID=<MASKED>"},
			{Re: re(`cc-socks/\d+\.sock`), With: "cc-socks/<MASKED>.sock"},
			{Re: re(`CLAUDE_CODE_MESSAGING_TOKEN=\S+`), With: "CLAUDE_CODE_MESSAGING_TOKEN=<MASKED>"},
			{Re: re(`CLAUDE_CODE_EXECPATH=\S+`), With: "CLAUDE_CODE_EXECPATH=<MASKED>"},
			// the per-user folder of the temp root, named for the uid the capture and the replay ran under
			{Re: re(`claude-\d+`), With: "claude-<UID>"},
			// the order the capture sanitised in: the repository first, as it holds the temp root
			{Re: re(regexp.QuoteMeta(repo)), With: "<RUN>"},
			{Re: re(regexp.QuoteMeta(work)), With: "<TMP>"},
			{Re: re(regexp.QuoteMeta(enc)), With: "<RUN_DIRNAME>"},
		},
		IDs: ids,
	}
}

// Dropped are the names Rules and Frames leave out of the comparison: object
// keys, assistant message keys that are dropped when empty, and the frames of
// the real stream that the mock never sends ("type/subtype"). No capability cell
// may name one (TestNoCellNamesWhatReplayDrops).
func Dropped() (keys, frames []string) {
	keys = append(keys, Rules("", "", nil).DropKeys...)
	keys = append(keys, "script", "mock_start_after_post", "caller", "thinking") // dropped from an Agent input, an assistant block, a block type
	for k := range assistantMeta {
		if k != "id" && k != "model" && k != "type" { // the response's own id, model and type: common words no cell is about
			keys = append(keys, k)
		}
	}
	for k := range assistantEmpty {
		keys = append(keys, k)
	}
	for f := range unmodelled {
		frames = append(frames, f)
	}
	return keys, frames
}
