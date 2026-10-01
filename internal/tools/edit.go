package tools

import (
	"errors"
	"strings"
)

// Edit errors: an exact-string replacement that cannot apply.
var (
	// ErrNoMatch: the string to replace does not appear in the file.
	ErrNoMatch = errors.New("the string to replace is not in the file")
	// ErrAmbiguous: it appears more than once and the edit did not ask to
	// replace them all.
	ErrAmbiguous = errors.New("the string to replace appears more than once")
)

// Edit replaces old with new in content, as an exact string, not a pattern. It
// must appear in content; unless replaceAll it must appear exactly once, and
// matches is how many times it did. With replaceAll every occurrence is
// replaced.
//
// sr:capability file-tools
func Edit(content, old, new string, replaceAll bool) (updated string, matches int, err error) {
	if old != "" {
		matches = strings.Count(content, old)
	}
	switch {
	case matches == 0:
		return content, matches, ErrNoMatch
	case matches > 1 && !replaceAll:
		return content, matches, ErrAmbiguous
	case replaceAll:
		return strings.ReplaceAll(content, old, new), matches, nil
	}
	return strings.Replace(content, old, new, 1), matches, nil
}

// Hunk is one run of changed lines with the lines around it, as a unified diff
// holds it. Lines carry their marker: " " for unchanged, "-" for removed, "+"
// for added.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Lines              []string
}

// patchContext is how many unchanged lines a hunk keeps on each side.
const patchContext = 3

// Patch is how newText differs from oldText, as a single hunk spanning the
// lines between their common first and last lines, with patchContext lines of
// context around it; no hunk when they are equal. A file's final newline does
// not start a line of its own.
func Patch(oldText, newText string) []Hunk {
	o, n := patchLines(oldText), patchLines(newText)
	pre := 0
	for pre < len(o) && pre < len(n) && o[pre] == n[pre] {
		pre++
	}
	suf := 0
	for suf < len(o)-pre && suf < len(n)-pre && o[len(o)-1-suf] == n[len(n)-1-suf] {
		suf++
	}
	if pre == len(o) && pre == len(n) {
		return nil
	}
	before, after := min(pre, patchContext), min(suf, patchContext)
	h := Hunk{OldStart: pre - before + 1, NewStart: pre - before + 1}
	for _, l := range o[pre-before : pre] {
		h.Lines = append(h.Lines, " "+l)
	}
	for _, l := range o[pre : len(o)-suf] {
		h.Lines = append(h.Lines, "-"+l)
	}
	for _, l := range n[pre : len(n)-suf] {
		h.Lines = append(h.Lines, "+"+l)
	}
	for _, l := range o[len(o)-suf : len(o)-suf+after] {
		h.Lines = append(h.Lines, " "+l)
	}
	h.OldLines = before + (len(o) - pre - suf) + after
	h.NewLines = before + (len(n) - pre - suf) + after
	return []Hunk{h}
}

func patchLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
