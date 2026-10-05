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
			"uuid", "request_id", "timestamp", // ids and times that differ in every run
			"usage", "modelUsage", "total_cost_usd", "duration_ms", "duration_api_ms", // the model's cost: the mock has no model
			"signature",                                                              // the model's thinking, signed
			"first_content_frame_ms", "fast_mode_state", "fast_mode_disabled_reason", // the real service's latency and mode
		},
		// the order the capture sanitised in: the repository first, as it holds the temp root
		Scrub: []rp.Scrub{
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
	keys = append(keys, "script", "caller", "thinking") // dropped from an Agent input, an assistant block, a block type
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
