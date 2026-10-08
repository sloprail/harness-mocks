package replay

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// stepSpecs are the runs of claude the setup holds, the run's own first. A later run is a directory
// then/NN (prompt.txt, args, cwd, symlink, and a settings.json and hook.sh of its own), or the flat
// files then-NN-prompt.txt, then-NN-args and then-NN-cwd; nothing else is installed.
func stepSpecs(setup string) ([]stepSpec, error) {
	first, err := parseStepArgs(readFile(filepath.Join(setup, "args")))
	if err != nil {
		return nil, err
	}
	first.prompt = strings.TrimSpace(readFile(filepath.Join(setup, "prompt.txt")))
	first.cwd = strings.TrimSpace(readFile(filepath.Join(setup, "cwd")))
	first.symlink = strings.TrimSpace(readFile(filepath.Join(setup, "symlink")))
	first.settings, first.hook = readFile(filepath.Join(setup, "settings.json")), readFile(filepath.Join(setup, "hook.sh"))
	specs := []stepSpec{first}
	type later struct {
		order string
		spec  stepSpec
	}
	var laters []later
	dirs, _ := filepath.Glob(filepath.Join(setup, "then", "*"))
	for _, d := range dirs {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			switch e.Name() {
			case "prompt.txt", "args", "cwd", "symlink", "settings.json", "hook.sh":
			default:
				return nil, unbuildable(fmt.Errorf("a later run has %s, which the adapter does not install", e.Name()))
			}
		}
		s, err := parseStepArgs(readFile(filepath.Join(d, "args")))
		if err != nil {
			return nil, err
		}
		s.prompt = strings.TrimSpace(readFile(filepath.Join(d, "prompt.txt")))
		s.cwd = strings.TrimSpace(readFile(filepath.Join(d, "cwd")))
		s.symlink = strings.TrimSpace(readFile(filepath.Join(d, "symlink")))
		s.settings, s.hook = readFile(filepath.Join(d, "settings.json")), readFile(filepath.Join(d, "hook.sh"))
		laters = append(laters, later{filepath.Join(d, "prompt.txt"), s})
	}
	flat, _ := filepath.Glob(filepath.Join(setup, "then-*-prompt.txt"))
	for _, p := range flat {
		base := strings.TrimSuffix(p, "prompt.txt")
		s, err := parseStepArgs(readFile(base + "args"))
		if err != nil {
			return nil, err
		}
		s.prompt = strings.TrimSpace(readFile(p))
		s.cwd = strings.TrimSpace(readFile(base + "cwd"))
		laters = append(laters, later{p, s})
	}
	// the capture runs the directories' steps, then the flat files'; each in name order
	sort.SliceStable(laters, func(i, j int) bool {
		di, dj := strings.Contains(laters[i].order, "/then/"), strings.Contains(laters[j].order, "/then/")
		return di != dj && di || di == dj && laters[i].order < laters[j].order
	})
	for _, l := range laters {
		specs = append(specs, l.spec)
	}
	return specs, nil
}
