package replay

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// runSteps runs the mock once for the recording's own prompt and once for each later run of claude
// (a resume), in the same repository and home, so a later run finds the session an earlier one
// left; it returns what they streamed, one after the other as the recording's stream is.
func runSteps(ctx context.Context, mock string, s Scenario, rec core.Recording, work, repo string, env []string) (string, error) {
	base := append([]string{mock, "-p", "--model", "haiku", "--dangerously-skip-permissions", "--output-format", "stream-json", "--verbose"}, mockFlags(work)...)
	type invocation struct {
		script string
		args   []string
		prompt string
	}
	first := strings.Fields(rec.Setup["args"])
	if !hasFlag(first, "--session-id") && !resumes(first) {
		first = append([]string{"--session-id", sessionID}, first...)
	}
	main := sessionID // what a later run's <SESSION> is: the id the first run started under
	if id := rec.Setup["session"]; id != "" {
		main = id
	}
	// @TRANSCRIPTS@ is where the sessions' transcripts are (the capture's own placeholder)
	transcripts := filepath.Join(work, "home", ".claude", "projects", regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(repo, "-"))
	for i, a := range first {
		first[i] = strings.ReplaceAll(a, "@TRANSCRIPTS@", transcripts)
	}
	runs := []invocation{{filepath.Join(work, "main.sh"), first, s.Prompt}}
	for i, st := range s.Then {
		var args []string
		for _, a := range st.Args {
			if a == "<SESSION>" {
				a = main
			}
			args = append(args, a)
		}
		script := filepath.Join(work, fmt.Sprintf("step%d.sh", i+1))
		if err := os.WriteFile(script, []byte(strings.NewReplacer("<RUN>", repo, "\\u003cRUN\\u003e", repo).Replace(st.Script)), 0o755); err != nil {
			return "", err
		}
		runs = append(runs, invocation{script, args, st.Prompt})
	}
	var out strings.Builder
	for _, r := range runs {
		argv := append(append(append([]string{}, base...), "--script", r.script), append(r.args, r.prompt)...)
		res, err := procexec.Run(ctx, procexec.Spec{Argv: argv, Dir: repo, Env: env, Timeout: 3 * time.Minute})
		if err != nil || res.ExitCode != wantExit(rec) || res.TimedOut {
			return "", &core.MockFailure{Detail: fmt.Sprintf("%v (exit %d): %s", err, res.ExitCode, res.Stderr)}
		}
		out.Write(res.Stdout)
	}
	return out.String(), nil
}

// mockFlags are the dirs the replay's mock is given: where claude keeps its config and its plugins
// (under the home a capture gives it, by default, so they are flags here, not environment).
func mockFlags(work string) []string {
	return []string{"--config-dir", filepath.Join(work, "home", ".claude"), "--plugin-cache-dir", filepath.Join(work, "plugins")}
}
