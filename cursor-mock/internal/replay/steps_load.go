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
func modelSteps(files map[string][]map[string]any, setup map[string]string, prompts []string, exits []int, heard []thoughtAt) ([]core.Agent, error) {
	dirs := []string{stepProject(setup, "")}
	for _, n := range stepNames(setup) {
		dirs = append(dirs, stepProject(setup, n))
	}
	segs, err := segments(files, dirs, prompts, exits)
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
		rec.Then = append(rec.Then, core.Step{Prompt: prompts[i], Agent: agents[i]})
	}
	return nil
}

// stepProject is the project directory a step ran from, by the name its transcripts
// are kept under: "repo" for the workspace, else the directory its then-<NN>-cwd names;
// "" stands for the first step.
func stepProject(setup map[string]string, name string) string {
	if cwd := strings.TrimSpace(setup["then-"+name+"-cwd"]); name != "" && cwd != "" {
		return cwd
	}
	return "repo"
}

// transcriptsOf are the main session's transcripts by the directory each step ran
// from: with several directories the harness keeps each under a folder of its own
// (<project>/<session>/<session>.jsonl), with one the session's folder is the
// transcript's own.
func transcriptsOf(dir, session string) (map[string][]map[string]any, error) {
	own, err := readJSONL(filepath.Join(dir, session, session+".jsonl"))
	if err != nil {
		return nil, err
	}
	if len(own) > 0 {
		return map[string][]map[string]any{"repo": own}, nil
	}
	found, _ := filepath.Glob(filepath.Join(dir, "*", session, session+".jsonl"))
	out := map[string][]map[string]any{}
	for _, f := range found {
		records, err := readJSONL(f)
		if err != nil {
			return nil, err
		}
		out[filepath.Base(filepath.Dir(filepath.Dir(f)))] = records
	}
	if len(out) == 0 {
		return nil, unbuildable("no transcript of the main session was recorded: the model's turns are unknown")
	}
	return out, nil
}
