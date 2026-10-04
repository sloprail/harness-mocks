package replay

import (
	"regexp"
	"strings"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// Rules are what a recording and a replay of it may differ in, and why:
// no capability cell is about any of it.
func Rules(repo, tmp string) rp.Rules {
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
			"aggregated_output": jobProcesses,
			"tool_response":     jobProcesses,
			// how the shell was invoked is the machine's: the command is what the model asked for
			"command": shellInner,
		},
		Scrub: []rp.Scrub{
			{Re: re(regexp.QuoteMeta(repo)), With: "<RUN>"},
			{Re: re(regexp.QuoteMeta(tmp)), With: "<TMP>"},
		},
		IDs: []*regexp.Regexp{re(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)}, // thread and session ids
	}
}

// shellInner is the command inside `/bin/<shell> -c[l] <command>`, whichever
// way the command was quoted (double quotes as codex writes it, single quotes
// as the mock does, none for a single word); a line that is not such an
// invocation is returned as it is.
func shellInner(line string) string {
	m := regexp.MustCompile(`^/bin/\w+ -l?c (.*)$`).FindStringSubmatch(line)
	if m == nil {
		return line
	}
	arg := m[1]
	switch {
	case strings.HasPrefix(arg, `"`) && strings.HasSuffix(arg, `"`):
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(arg[1 : len(arg)-1])
	case strings.HasPrefix(arg, "'") && strings.HasSuffix(arg, "'"):
		return strings.ReplaceAll(arg[1:len(arg)-1], `'\''`, "'")
	}
	return arg
}

// jobProcesses reduces the output of `ps ax` to the processes the replayed
// command started (`sleep N`, or a shell running it), without pids, ttys and
// times: the rest of the listing is the machine's (the test runner, the
// harness, other sessions), and not what any capability cell is about. Text
// that is not a `ps` listing is returned as it is.
func jobProcesses(text string) string {
	ps := regexp.MustCompile(`^\s*\d+\s+\S+\s+\S+\s+\d+:\d+(?:\.\d+)?\s+(.*)$`)
	job := regexp.MustCompile(`^(?:/bin/\w+ -c )?sleep \d+`)
	var jobs []string
	listing := false
	for _, l := range strings.Split(text, "\n") {
		if m := ps.FindStringSubmatch(l); m != nil {
			listing = true
			if job.MatchString(m[1]) {
				jobs = append(jobs, "<job> "+regexp.MustCompile(`^/bin/\w+ `).ReplaceAllString(m[1], "<SHELL> "))
			}
		}
	}
	if !listing {
		return text
	}
	return "<ps: " + strings.Join(jobs, " | ") + ">"
}
