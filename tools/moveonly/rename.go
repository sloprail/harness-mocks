package main

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// renames maps a package directory at base to its directory at head.
type renames map[string]string

// unit is one package to compare: directory u.base at base against u.head at head.
type unit struct{ base, head string }

func (u unit) relocated() bool { return u.base != u.head }

func (u unit) label() string {
	if u.relocated() {
		return u.base + " -> " + u.head
	}
	return u.base
}

func (r renames) String() string { return fmt.Sprint(map[string]string(r)) }

// Set implements flag.Value for a repeatable --rename old=new.
func (r *renames) Set(v string) error {
	from, to, ok := strings.Cut(v, "=")
	from, to = path.Clean(strings.TrimSpace(from)), path.Clean(strings.TrimSpace(to))
	if !ok || from == "" || to == "" || from == to || strings.HasPrefix(from+to, "..") || strings.HasPrefix(from+to, "/") {
		return fmt.Errorf("--rename wants <oldDir>=<newDir> (distinct, repo-relative), got %q", v)
	}
	if *r == nil {
		*r = renames{}
	}
	if _, dup := (*r)[from]; dup {
		return fmt.Errorf("--rename: %s given twice", from)
	}
	(*r)[from] = to
	return nil
}

func (r renames) units() []unit {
	var us []unit
	for from, to := range r {
		us = append(us, unit{from, to})
	}
	sort.Slice(us, func(i, j int) bool { return us[i].base < us[j].base })
	return us
}

// blurPackageName hides the package name in each item's file signature, for a
// package whose directory (and so its package clause) was renamed. The
// distinction between a package and its _test package is kept.
func blurPackageName(items []item) {
	for i, it := range items {
		name, rest, _ := strings.Cut(it.Sig, ";")
		blur := "PKG"
		if strings.HasSuffix(name, "_test") {
			blur = "PKG_test"
		}
		if rest != "" {
			blur += ";" + rest
		}
		items[i].Sig = blur
	}
	sortItems(items)
}
