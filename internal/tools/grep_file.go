package tools

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ByRecency orders the hits the newest file first, a tie by name, last first.
func (r GrepResult) ByRecency() []GrepHit {
	hits := append([]GrepHit(nil), r.Hits...)
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Modified != hits[j].Modified {
			return hits[i].Modified > hits[j].Modified
		}
		return hits[i].Name > hits[j].Name
	})
	return hits
}

func globMatches(globs []*regexp.Regexp, pattern, rel, name string) bool {
	subject := name
	if strings.Contains(pattern, "/") {
		subject = rel
	}
	for _, g := range globs {
		if g.MatchString(subject) {
			return true
		}
	}
	return false
}

// shown is a path as the search reports it: relative to dir when it lies under it.
func shown(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}

// grepFile is the matching lines of one file; false when it has none, is not readable or is binary.
func grepFile(re *regexp.Regexp, path string) (GrepHit, bool) {
	data, err := os.ReadFile(path)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return GrepHit{}, false
	}
	var hit GrepHit
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), len(data)+1)
	for n := 1; sc.Scan(); n++ {
		if re.MatchString(sc.Text()) {
			hit.Numbers = append(hit.Numbers, n)
			hit.Lines = append(hit.Lines, sc.Text())
		}
	}
	if len(hit.Numbers) == 0 {
		return GrepHit{}, false
	}
	if info, err := os.Stat(path); err == nil {
		hit.Modified = info.ModTime().UnixNano()
	}
	return hit, true
}
