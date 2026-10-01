package tools

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// GlobResult is the files a pattern found.
type GlobResult struct {
	// Names are the matching files, relative to the directory searched, the
	// oldest modification first, at most the limit asked for.
	Names []string
	// Total is how many files matched, whatever the limit.
	Total int
	// Truncated: more matched than Names holds.
	Truncated bool
}

// Glob finds the files under dir whose path relative to dir matches pattern:
// "*" and "?" stay within one path element, "**" crosses elements, and "{a,b}"
// is either. Directories are not results. At most limit names are returned
// (all when limit is zero), the oldest modification first.
func Glob(dir, pattern string, limit int) (GlobResult, error) {
	var res []*regexp.Regexp
	for _, p := range expandBraces(pattern) {
		re, err := regexp.Compile("^" + globRegexp(p) + "$")
		if err != nil {
			return GlobResult{}, err
		}
		res = append(res, re)
	}
	type found struct {
		name string
		mod  int64
	}
	var files []found
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		for _, re := range res {
			if re.MatchString(rel) {
				if info, ierr := d.Info(); ierr == nil {
					files = append(files, found{rel, info.ModTime().UnixNano()})
				}
				break
			}
		}
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool { return files[i].mod < files[j].mod })
	out := GlobResult{Total: len(files)}
	for i, f := range files {
		if limit > 0 && i >= limit {
			out.Truncated = true
			break
		}
		out.Names = append(out.Names, f.name)
	}
	return out, nil
}

// expandBraces turns "a.{x,y}" into "a.x" and "a.y" (nesting allowed).
func expandBraces(p string) []string {
	open := strings.Index(p, "{")
	if open < 0 {
		return []string{p}
	}
	depth, closeAt := 0, -1
	for i := open; i < len(p) && closeAt < 0; i++ {
		switch p[i] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				closeAt = i
			}
		}
	}
	if closeAt < 0 {
		return []string{p}
	}
	var alts []string
	depth, from := 0, open+1
	for i := open + 1; i < closeAt; i++ {
		switch p[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				alts = append(alts, p[from:i])
				from = i + 1
			}
		}
	}
	alts = append(alts, p[from:closeAt])
	var out []string
	for _, a := range alts {
		out = append(out, expandBraces(p[:open]+a+p[closeAt+1:])...)
	}
	return out
}

// globRegexp is the regular expression of one brace-free glob.
func globRegexp(p string) string {
	var b strings.Builder
	for i := 0; i < len(p); i++ {
		switch c := p[i]; {
		case c == '*' && i+1 < len(p) && p[i+1] == '*':
			i++
			if i+1 < len(p) && p[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}
