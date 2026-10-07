package tools

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Output modes of a search.
const (
	// GrepFiles lists the files that match (the default).
	GrepFiles = "files_with_matches"
	// GrepContent lists the matching lines.
	GrepContent = "content"
	// GrepCount counts the matching lines of each file.
	GrepCount = "count"
)

// ErrNoSuchPath: the place to search is not there.
var ErrNoSuchPath = errors.New("the path to search does not exist")

// GrepQuery is a search of file contents.
type GrepQuery struct {
	// Pattern is a regular expression, matched within one line.
	Pattern string
	// Dir is the directory the search runs in; Path, when set, is the file or
	// directory searched, relative to Dir (or absolute).
	Dir, Path string
	// Glob narrows the files searched: a pattern without a slash matches a file's name, one
	// with a slash its path under the searched directory.
	Glob string
	// IgnoreCase matches without regard to case.
	IgnoreCase bool
}

// GrepHit is the matching lines of one file.
type GrepHit struct {
	// Name is the file as it is shown: relative to Dir when it lies under it.
	Name string
	// Numbers and Lines are the matching lines' numbers (from 1) and their text.
	Numbers []int
	Lines   []string
	// Modified is the file's modification time, in nanoseconds.
	Modified int64
}

// GrepResult is what a search found.
type GrepResult struct {
	// Hits are the files with a match, by path.
	Hits []GrepHit
	// SingleFile: the search named one file, whose lines are shown without its name.
	SingleFile bool
}

// Grep searches the files under the query's path for lines matching its pattern. Hidden files and
// directories, and files with a NUL byte (binary files), are not searched. It returns *ParseError for a
// pattern that does not parse and ErrNoSuchPath for a path that is not there.
//
// sr:capability grep-tool
func Grep(q GrepQuery) (GrepResult, error) {
	re, err := compileGrep(q.Pattern, q.IgnoreCase)
	if err != nil {
		return GrepResult{}, err
	}
	globs, err := compileGlobs(q.Glob)
	if err != nil {
		return GrepResult{}, err
	}
	root := q.Dir
	if q.Path != "" {
		root = q.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(q.Dir, root)
		}
	}
	info, err := os.Stat(root)
	if err != nil {
		return GrepResult{}, ErrNoSuchPath
	}
	res := GrepResult{SingleFile: !info.IsDir()}
	search := func(path, name string) {
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil || res.SingleFile {
			rel = name
		}
		if len(globs) > 0 && !globMatches(globs, q.Glob, filepath.ToSlash(rel), name) {
			return
		}
		if hit, ok := grepFile(re, path); ok {
			hit.Name = shown(q.Dir, path)
			res.Hits = append(res.Hits, hit)
		}
	}
	if res.SingleFile {
		search(root, info.Name())
	} else {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return nil
			case path != root && strings.HasPrefix(d.Name(), "."):
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			case !d.IsDir():
				search(path, d.Name())
			}
			return nil
		})
	}
	sort.SliceStable(res.Hits, func(i, j int) bool { return res.Hits[i].Name < res.Hits[j].Name })
	return res, nil
}
