// Package session is the harness-neutral core of finding, resuming and forking
// a session's transcript.
package session

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/transcript"
)

// Exists reports whether p is a regular file.
func Exists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// Find is the existing transcript of session id: under cwd's project
// directory, else in any other project directory (a session begun in another
// directory is resumed all the same); the most recently written wins when
// several hold it. It is "" when none does, and for an id that is not a plain
// file name.
func Find(l transcript.Layout, configDir, cwd, id string) string {
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return ""
	}
	if p := l.FilePath(configDir, cwd, id); Exists(p) {
		return p
	}
	entries, err := os.ReadDir(filepath.Join(configDir, l.ProjectsDir))
	if err != nil {
		return ""
	}
	best := ""
	var bestMod int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(configDir, l.ProjectsDir, e.Name(), id+l.Ext)
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if best == "" || fi.ModTime().UnixNano() > bestMod {
			best, bestMod = p, fi.ModTime().UnixNano()
		}
	}
	return best
}
