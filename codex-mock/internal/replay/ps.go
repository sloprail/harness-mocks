package replay

import (
	"regexp"
	"strings"
)

var psLine = regexp.MustCompile(`^\s*\d+\s+\S+\s+\S+\s+\d+:\d+(?:\.\d+)?\s+(.*)$`)

// scratchText is text a command printed with what is the machine's taken out: a
// `ps` listing reduced to the job's own processes, and the commit a scratch
// repository's checkout names (its id depends on the time it was made), and
// where the harness's own npm package is installed.
func scratchText(text string) string {
	lines := strings.Split(jobProcesses(text), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "CODEX_MANAGED_PACKAGE_ROOT=") { // where the harness's own npm package is installed
			lines[i] = "CODEX_MANAGED_PACKAGE_ROOT=<PKG_ROOT>"
		}
		if rest, ok := strings.CutPrefix(l, "HEAD is now at "); ok {
			if _, subject, found := strings.Cut(rest, " "); found {
				lines[i] = "HEAD is now at <SHA> " + subject
			}
		}
	}
	return strings.Join(lines, "\n")
}

// jobProcesses reduces the output of `ps ax` to the processes the replayed
// command started (`sleep N`, or a shell running it), without pids, ttys and
// times: the rest of the listing is the machine's (the test runner, the
// harness, other sessions), and not what any capability cell is about. Text
// that is not a `ps` listing is returned as it is. What a command line is
// (a shell invocation, a sleep) is read by parsing it as shell (shell.go).
func jobProcesses(text string) string {
	var jobs []string
	listing := false
	for _, l := range strings.Split(text, "\n") {
		if m := psLine.FindStringSubmatch(l); m != nil {
			listing = true
			if isJob(m[1]) {
				jobs = append(jobs, "<job> "+jobLabel(m[1]))
			}
		}
	}
	if !listing {
		return text
	}
	return "<ps: " + strings.Join(jobs, " | ") + ">"
}
