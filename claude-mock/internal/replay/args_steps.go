package replay

import (
	"fmt"
	"strings"
)

// stepSpec is one run of claude the recording holds: the run's own (setup/prompt.txt, setup/args)
// and each later one (setup/then/NN/prompt.txt, args), in name order.
type stepSpec struct {
	prompt string
	args   []string // the flags the mock models (parseArgs)
	// how the run starts its session: resume names a session, cont is --continue, newID is
	// --session-id (a run with none starts the replay's own session)
	resume, newID string
	cont, fork    bool
}

// parseStepArgs reads a later run's args: the session flags are its own, the rest are parseArgs's.
func parseStepArgs(text string) (stepSpec, error) {
	var s stepSpec
	var rest []string
	f := strings.Fields(text)
	for i := 0; i < len(f); i++ {
		switch f[i] {
		case "--continue":
			s.cont = true
		case "--resume", "--session-id":
			if i+1 >= len(f) {
				return s, unbuildable(fmt.Errorf("the args end after %s, short of its value", f[i]))
			}
			if f[i] == "--resume" {
				s.resume = f[i+1]
			} else {
				s.newID = f[i+1]
			}
			i++
		case "--fork-session":
			s.fork = true
		default:
			rest = append(rest, f[i])
		}
	}
	args, err := parseArgs(strings.Join(rest, "\n"))
	s.args = args
	return s, err
}
