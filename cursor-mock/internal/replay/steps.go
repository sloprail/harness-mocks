package replay

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// A run of several steps (recorded: runs/session-fork) is one cursor-agent run
// after another over the same home: the scenario's own prompt.txt is the first, and
// each setup/then-<NN>-prompt.txt (with then-<NN>-args, then-<NN>-cwd) a later one,
// in name order. exit.txt has one line per step.

// stepFile reports whether a setup file belongs to a later step.
func stepFile(name string) bool {
	rest, ok := strings.CutPrefix(name, "then-")
	if !ok {
		return false
	}
	for _, suffix := range []string{"-prompt.txt", "-args", "-cwd"} {
		if n, ok := strings.CutSuffix(rest, suffix); ok && n != "" && strings.Trim(n, "0123456789") == "" {
			return true
		}
	}
	return false
}

// stepNames are the numbers of the later steps (as their files write them), in order.
func stepNames(setup map[string]string) []string {
	var names []string
	for name := range setup {
		if rest, ok := strings.CutPrefix(name, "then-"); ok && strings.HasSuffix(name, "-prompt.txt") {
			names = append(names, strings.TrimSuffix(rest, "-prompt.txt"))
		}
	}
	sort.Strings(names)
	return names
}

// exitsOf are the exit statuses exit.txt records, one per step.
func exitsOf(text string) ([]int, error) {
	var out []int
	for _, l := range strings.Fields(text) {
		n, err := strconv.Atoi(l)
		if err != nil {
			return nil, unbuildable("exit.txt holds %q", l)
		}
		out = append(out, n)
	}
	return out, nil
}

// segments cut a session's transcripts into the records of each step. A transcript is
// kept per project directory (the one the step ran from): a step begins at the user
// record that asks its prompt, among the steps of its directory, the first at the
// transcript's start, and a step cursor-agent refused (a status that is not 0) has none.
func segments(files map[string][]map[string]any, dirs []string, prompts []string, exits []int) ([][]map[string]any, error) {
	segs := make([][]map[string]any, len(prompts))
	for dir, records := range files {
		var mine []int // the steps that ran from this directory and ended well, in order
		for i := range prompts {
			if dirs[i] == dir && exits[i] == 0 {
				mine = append(mine, i)
			}
		}
		if len(mine) == 0 {
			return nil, unbuildable("a transcript is kept for the directory %q, which no step ran from", dir)
		}
		cur, next := mine[0], 1
		for _, rec := range records {
			if rec["role"] == "user" && next < len(mine) && firstQuery([]map[string]any{rec}) == prompts[mine[next]] {
				cur, next = mine[next], next+1
			}
			segs[cur] = append(segs[cur], rec)
		}
	}
	for i, s := range segs {
		if len(s) == 0 && exits[i] == 0 {
			return nil, unbuildable("the transcript holds nothing of step %d, which ended well", i)
		}
	}
	return segs, nil
}

// requestsIn is how many model requests a stretch of a transcript holds: the runs
// of responses between user records.
func requestsIn(records []map[string]any) int {
	n, fresh := 0, true
	for _, rec := range records {
		switch rec["role"] {
		case "user":
			fresh = true
		case "assistant":
			if fresh {
				n++
				fresh = false
			}
		}
	}
	return n
}

// rebase is the thoughts that were had in the n requests from the one numbered
// from, numbered from 0 again.
func rebase(heard []thoughtAt, from, n int) []thoughtAt {
	var out []thoughtAt
	for _, h := range heard {
		if h.request >= from && h.request < from+n {
			h.request -= from
			out = append(out, h)
		}
	}
	return out
}

// stepDir is where a later step runs: the workspace, or the directory its then-<NN>-cwd
// names, next to it.
func stepDir(work, repo string, setup map[string]string, name string) string {
	if cwd := strings.TrimSpace(setup["then-"+name+"-cwd"]); cwd != "" {
		return filepath.Join(work, cwd)
	}
	return repo
}
