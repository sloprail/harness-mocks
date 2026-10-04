package replay

import (
	"regexp"
	"strings"

	rp "github.com/sloprail/harness-mocks/internal/replay"
)

// Rules are what a recording and a replay of it may differ in, and why:
// no capability cell is about any of it.
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
			"aggregated_output": jobProcesses,
			"tool_response":     jobProcesses,
			// how the shell was invoked is the machine's: the command is what the model asked for
			"command": shellInner,
			// where the harness's own npm package is installed is the machine's
			"CODEX_MANAGED_PACKAGE_ROOT": func(string) string { return "<PKG_ROOT>" },
		},
		Scrub: []rp.Scrub{
			{Re: re(regexp.QuoteMeta(repo)), With: "<RUN>"},
			{Re: re(regexp.QuoteMeta(root)), With: "<TMP>"},                         // the run's scratch directory: the repository, CODEX_HOME and TMPDIR sit in it
			{Re: re(`HEAD is now at [0-9a-f]{7,} `), With: "HEAD is now at <SHA> "}, // the scratch repository's commit
			// where the harness's own npm package is installed: the machine's
			{Re: re(`(CODEX_MANAGED_PACKAGE_ROOT"?[=:]"?)[^"\n,}]*`), With: "${1}<PKG_ROOT>"},
			{Re: re(`"nickname":"[^"]*"`), With: `"nickname":"<NICKNAME>"`}, // the name Codex picks for a sub-agent
		},
		IDs: []*regexp.Regexp{re(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)}, // thread and session ids
	}
}

// shellInner is the command inside `/bin/<shell> -c[l] <command>`, however the
// command was quoted (codex's display of it mixes quotes: `"a '"'b'"' c"`; the
// mock's uses single quotes); a line that is not such an invocation is returned
// as it is.
func shellInner(line string) string {
	words, ok := shellWords(line)
	if !ok || len(words) < 3 || !regexp.MustCompile(`^/bin/\w+$`).MatchString(words[0]) || (words[1] != "-c" && words[1] != "-lc") {
		return line
	}
	return strings.Join(words[2:], " ")
}

// shellWords splits a command line into words as a POSIX shell does: whitespace
// separates, single quotes keep everything, double quotes keep everything but
// \" and \\, and adjoining quoted pieces make one word.
func shellWords(line string) (words []string, ok bool) {
	var cur strings.Builder
	in := false // a word has begun (an empty quoted word is one)
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == ' ' || c == '\t':
			if in {
				words, in = append(words, cur.String()), false
				cur.Reset()
			}
		case c == '\'':
			j := strings.IndexByte(line[i+1:], '\'')
			if j < 0 {
				return nil, false
			}
			cur.WriteString(line[i+1 : i+1+j])
			i, in = i+1+j, true
		case c == '"':
			in = true
			for i++; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) && strings.IndexByte("\"\\$`", line[i+1]) >= 0 {
					i++
				}
				cur.WriteByte(line[i])
			}
			if i >= len(line) {
				return nil, false
			}
		case c == '\\' && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
			in = true
		default:
			cur.WriteByte(c)
			in = true
		}
	}
	if in {
		words = append(words, cur.String())
	}
	return words, true
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
