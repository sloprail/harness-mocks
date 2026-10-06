package replay

import (
	"path/filepath"
	"strings"

	core "github.com/sloprail/harness-mocks/internal/replay"
)

// stepsOf reads the run's steps' prompts and exit statuses (exit.txt, one line per
// step); a run whose first step failed has no turns of the model to replay.
func stepsOf(rec *core.Recording, sample string) (prompts []string, exits []int, err error) {
	rec.Setup["exit"] = readFile(filepath.Join(sample, "exit.txt"))
	if exits, err = exitsOf(rec.Setup["exit"]); err != nil {
		return nil, nil, err
	}
	prompts = []string{rec.Prompt}
	for _, n := range stepNames(rec.Setup) {
		prompts = append(prompts, strings.TrimRight(rec.Setup["then-"+n+"-prompt.txt"], "\n"))
	}
	if len(exits) != len(prompts) {
		return nil, nil, unbuildable("exit.txt has %d lines for %d steps", len(exits), len(prompts))
	}
	if exits[0] != 0 {
		return nil, nil, unbuildable("the first step ended with status %d: the model's turns are not told", exits[0])
	}
	return prompts, exits, nil
}

// modelSteps are the model's turns in each step of the transcript; a step that
// ended with a status other than 0 has none.
func modelSteps(records []map[string]any, prompts []string, exits []int, heard []thoughtAt) ([]core.Agent, error) {
	segs, err := segments(records, prompts, exits)
	if err != nil {
		return nil, err
	}
	agents := make([]core.Agent, len(prompts))
	before := 0
	for i, seg := range segs {
		n := requestsIn(seg)
		if len(seg) > 0 {
			if agents[i], err = modelTurns(seg, rebase(heard, before, n)); err != nil {
				return nil, unbuildable("%v", err)
			}
		}
		before += n
	}
	return agents, nil
}

// addLater puts the steps after the first on the recording. A later step's
// sub-agents are not attached to their transcripts, so one that starts any is not
// replayed.
func addLater(rec *core.Recording, prompts []string, agents []core.Agent) error {
	for i := 1; i < len(agents); i++ {
		for _, c := range agents[i].Calls {
			if c.Tool == core.ToolSpawn {
				return unbuildable("step %d starts a sub-agent: only the first step's are attached to their transcripts", i)
			}
		}
		rec.Later = append(rec.Later, core.Step{Prompt: prompts[i], Agent: agents[i]})
	}
	return nil
}
