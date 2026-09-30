package main

import (
	"path"
	"sort"
	"strings"
)

// pkgResult is the comparison of one directory between two revisions.
type pkgResult struct {
	dir                string
	count              int
	onlyBase, onlyHead []item
	notes              []string
}

func (p pkgResult) ok() bool {
	return len(p.onlyBase)+len(p.onlyHead)+len(p.notes) == 0
}

// outcome is the verdict for one base..head pair.
type outcome struct {
	nonGo []string
	pkgs  []pkgResult
}

func (o outcome) ok() bool {
	if len(o.nonGo) > 0 {
		return false
	}
	for _, p := range o.pkgs {
		if !p.ok() {
			return false
		}
	}
	return true
}

func checkPair(r repo, base, head string) (outcome, error) {
	var o outcome
	changed, err := r.changed(base, head)
	if err != nil {
		return o, err
	}
	dirs := map[string]bool{}
	for _, f := range changed {
		if strings.HasSuffix(f, ".go") {
			dirs[path.Dir(f)] = true
		} else {
			o.nonGo = append(o.nonGo, f)
		}
	}
	var names []string
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)
	for _, dir := range names {
		b, err := dumpDir(r, base, dir)
		if err != nil {
			return o, err
		}
		h, err := dumpDir(r, head, dir)
		if err != nil {
			return o, err
		}
		res := pkgResult{dir: dir, count: len(h.items)}
		res.onlyBase, res.onlyHead = diffItems(b.items, h.items)
		for f, pkg := range b.pkgs {
			if hp, ok := h.pkgs[f]; ok && hp != pkg {
				res.notes = append(res.notes, "package name of "+f+" changed: "+pkg+" -> "+hp)
			}
		}
		sort.Strings(res.notes)
		o.pkgs = append(o.pkgs, res)
	}
	markCrossDir(o.pkgs)
	return o, nil
}

// markCrossDir notes declarations that left one directory and appeared in another.
func markCrossDir(pkgs []pkgResult) {
	added := map[string]string{}
	for _, p := range pkgs {
		for _, it := range p.onlyHead {
			added[it.Kind+it.Text] = p.dir
		}
	}
	for i, p := range pkgs {
		for _, it := range p.onlyBase {
			if to, ok := added[it.Kind+it.Text]; ok && to != p.dir {
				pkgs[i].notes = append(pkgs[i].notes, "moved across directories: "+p.dir+" -> "+to+": "+it.String())
			}
		}
	}
}
