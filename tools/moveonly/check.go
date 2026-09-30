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

func checkPair(r repo, base, head string, rn renames) (outcome, error) {
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
	for _, u := range rn.units() {
		delete(dirs, u.base)
		delete(dirs, u.head)
	}
	units := rn.units()
	for d := range dirs {
		units = append(units, unit{d, d})
	}
	sort.Slice(units, func(i, j int) bool { return units[i].label() < units[j].label() })
	for _, u := range units {
		res, err := checkUnit(r, base, head, u)
		if err != nil {
			return o, err
		}
		o.pkgs = append(o.pkgs, res)
	}
	markCrossDir(o.pkgs)
	return o, nil
}

// checkUnit compares one package: u.base at base against u.head at head.
func checkUnit(r repo, base, head string, u unit) (pkgResult, error) {
	b, err := dumpDir(r, base, u.base)
	if err != nil {
		return pkgResult{}, err
	}
	h, err := dumpDir(r, head, u.head)
	if err != nil {
		return pkgResult{}, err
	}
	res := pkgResult{dir: u.label(), count: len(h.items)}
	if u.relocated() {
		if len(b.items)+len(h.items) == 0 {
			res.notes = append(res.notes, "--rename names directories with no Go files")
		}
		if path.Base(u.base) != path.Base(u.head) {
			blurPackageName(b.items)
			blurPackageName(h.items)
		}
	} else {
		for f, pkg := range b.pkgs {
			if hp, ok := h.pkgs[f]; ok && hp != pkg {
				res.notes = append(res.notes, "package name of "+f+" changed: "+pkg+" -> "+hp)
			}
		}
	}
	res.onlyBase, res.onlyHead = diffItems(b.items, h.items)
	sort.Strings(res.notes)
	return res, nil
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
