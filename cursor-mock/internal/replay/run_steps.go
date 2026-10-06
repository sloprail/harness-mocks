package replay

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sloprail/harness-mocks/internal/procexec"
	core "github.com/sloprail/harness-mocks/internal/replay"
)

// runSteps runs the mock once for each step of the run, one after the other over
// the same home, as the capture runs cursor-agent: the first with the scenario's
// own flags, each later one with its own (<SESSION> is the first step's session).
// It returns the steps' stdout together and the status each ended with: a step the
// mock refuses is behaviour to compare (the status), not a failure of the mock;
// only a step that could not be run, or ran out of time, is.
func (a Adapter) runSteps(mock string, rec core.Recording, l layout, flags []string) (stdout string, exits []int, err error) {
	base := []string{mock, "-p"}
	if _, noForce := rec.Setup["no-force"]; !noForce {
		base = append(base, "--force")
	}
	base = append(base, "--trust", "--model", "auto", "--output-format", "stream-json")
	steps := append([]laterStep{{script: l.main, prompt: rec.Prompt, dir: l.cwd, flags: flags}}, l.later...)
	session := ""
	var out strings.Builder
	for i, st := range steps {
		argv := append([]string(nil), base...)
		for _, f := range st.flags {
			if f == "<SESSION>" {
				if session == "" {
					return "", nil, &core.MockFailure{Detail: "step 1 named no session for a later step to resume"}
				}
				f = session
			}
			argv = append(argv, f)
		}
		argv = append(argv, "--script", st.script, st.prompt)
		res, err := procexec.Run(context.Background(), procexec.Spec{Argv: argv, Dir: st.dir, Env: l.env, Timeout: 2 * time.Minute})
		if err != nil || res.TimedOut || res.ExitCode < 0 {
			return "", nil, &core.MockFailure{Detail: fmt.Sprintf("step %d: %v (exit %d): %s", i+1, err, res.ExitCode, res.Stderr)}
		}
		out.Write(res.Stdout)
		exits = append(exits, res.ExitCode)
		if i == 0 {
			session = sessionOfOutput(string(res.Stdout))
		}
	}
	return out.String(), exits, nil
}

// sessionOfOutput is the session id the mock's stream names.
func sessionOfOutput(text string) string {
	frames, _ := parseJSONL(text)
	return sessionOf(frames)
}
