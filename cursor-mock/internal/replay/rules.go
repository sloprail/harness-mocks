package replay

import (
	"os/user"
	"regexp"
	"strconv"
	"strings"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// Rules are what a recording and a replay of it may differ in, and why: no
// capability cell is about any of it (a test checks that). repo and work are the
// replay's own paths, which the recording has as the capture wrote them: <RUN>,
// <TMP>, and the run directory as Cursor encodes it into a folder name,
// <RUN_DIRNAME>.
func Rules(repo, work string) rp.Rules {
	re := regexp.MustCompile
	scrubs := []rp.Scrub{}
	if u, err := user.Current(); err == nil && u.HomeDir != "" && u.HomeDir != "/" { // where the harness is installed: the account's home, which the capture wrote as <HOME>
		scrubs = append(scrubs, rp.Scrub{Re: re(regexp.QuoteMeta(u.HomeDir) + `(/\.local/share/cursor-agent/)`), With: "<HOME>$1"})
	}
	return rp.Rules{
		DropKeys: dropKeys,
		Measured: measured,
		Rewrite: map[string]func(string) string{
			"durationMs": measuredText, "runtimeMs": measuredText,
			"startedAtMs": measuredText, "completedAtMs": measuredText, // when a call began and ended: that it says so, not when
			"model_call_id": masked("<model call>"), "request_id": masked("<request>"), // the service's ids of its own calls: that they are named
			"task_id": maskedNumber("<shell>"), "taskId": maskedNumber("<shell>"), // a background shell's id is a number of the harness's own
		},
		// the order the capture sanitised in: the repository first, as it holds the temp root
		Scrub: append([]rp.Scrub{
			{Re: re(regexp.QuoteMeta(repo)), With: "<RUN>"},
			{Re: re(regexp.QuoteMeta(work)), With: "<TMP>"},
			{Re: re(regexp.QuoteMeta(encode(repo))), With: "<RUN_DIRNAME>"},
			// the shell id and the pid a background command was given, and where its output
			// is kept (named by the id): the harness's own numbers, that they are there is
			// what is compared
			{Re: re(`"shell_id":[0-9]+`), With: `"shell_id":<shell>`},
			{Re: re(`"pid":[0-9]+`), With: `"pid":<pid>`},
			{Re: re(`Shell ID: [0-9]+`), With: "Shell ID: <shell>"},
			{Re: re(`PID: [0-9]+`), With: "PID: <pid>"},
			{Re: re(`/terminals/[0-9]+\.txt`), With: "/terminals/<shell>.txt"},
			// a response's number and four characters after the request's id name it (the
			// generation of a thought): the run's own, not behaviour
			{Re: re(`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})-[0-9]+-[a-z0-9]{4}\b`), With: "$1-<n>-<name>"},
			// what an invalid model is answered with lists the models the account has: the
			// real service's catalogue, which the mock has none of
			{Re: re(`(?s)(Allowed model slugs:).*`), With: "$1 <the models available>"},
		}, scrubs...),
		IDs: []*regexp.Regexp{
			re(`call-[0-9a-f-]{36}-[0-9]+\nfc_[A-Za-z0-9_-]+`),                 // a real call's id: before the uuid it holds
			re(`toolu_[A-Za-z0-9_]+`),                                          // a scripted call's id
			re(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), // session, generation and agent ids
		},
	}
}

// encode is a path as Cursor names its project folder: every character that is
// not a letter or a digit as "-", the leading slash dropped.
func encode(path string) string {
	return regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(path[1:], "-")
}

// dropKeys are what differs in every run whatever the model does: the clock
// and what the real service reports of its own calls. Each is dropped wherever
// it occurs.
var dropKeys = []string{
	"usage", // the model's token counts: the mock has no model
}

// measured are the numbers that say how long something took: the cells that
// speak of them ask that they be there and positive, not what they were.
var measured = []string{"pid", "shellId", "duration", "duration_ms", "duration_api_ms", "executionTime", "localExecutionTimeMs", "timestamp_ms"}

// masked is a rewrite that says a value is there and not what it is.
func masked(with string) func(string) string {
	return func(s string) string {
		if s == "" {
			return s
		}
		return with
	}
}

// maskedNumber is a rewrite that says a value made of digits is there and not
// what it is; any other value is left alone.
func maskedNumber(with string) func(string) string {
	return func(s string) string {
		if s == "" || strings.Trim(s, "0123456789") != "" {
			return s
		}
		return with
	}
}

// measuredText is a duration the real stream writes as a string of digits, as
// measure says it of a number.
func measuredText(s string) string {
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return rp.Measure(n)
	}
	return s
}
