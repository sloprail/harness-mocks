package main

import "fmt"

// pkgDump is the sorted multiset of items of one directory at one revision,
// plus the package name of each file.
type pkgDump struct {
	items []item
	pkgs  map[string]string // file path -> package name
}

func dumpDir(r repo, rev, dir string) (pkgDump, error) {
	d := pkgDump{pkgs: map[string]string{}}
	files, err := r.goFiles(rev, dir)
	if err != nil {
		return d, err
	}
	for _, f := range files {
		src, err := r.blob(rev, f)
		if err != nil {
			return d, err
		}
		items, pkg, err := parseItems(f, src)
		if err != nil {
			return d, fmt.Errorf("%s at %s: %w", f, rev, err)
		}
		d.items = append(d.items, items...)
		d.pkgs[f] = pkg
	}
	sortItems(d.items)
	return d, nil
}

// diffItems merge-joins two sorted item lists and returns what only base has
// and what only head has.
func diffItems(base, head []item) (onlyBase, onlyHead []item) {
	i, j := 0, 0
	for i < len(base) || j < len(head) {
		switch {
		case j == len(head) || (i < len(base) && base[i].key() < head[j].key()):
			onlyBase = append(onlyBase, base[i])
			i++
		case i == len(base) || head[j].key() < base[i].key():
			onlyHead = append(onlyHead, head[j])
			j++
		default:
			i++
			j++
		}
	}
	return
}
