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
//
// sr:capability session-resume
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

// Target is the session id a harness's `--resume` value names: the path of a
// transcript file (its file name, without the layout's extension, is the id), the
// id of a session Find finds, or the name of a session, which named reports for
// the transcript at the path it is given; the most recently written named session
// wins. A value that is none of them is returned as it is, for the caller to fail
// on as an unknown session.
func Target(l transcript.Layout, configDir, cwd, value string, named func(path string) bool) string {
	if strings.HasSuffix(value, l.Ext) || strings.ContainsRune(value, filepath.Separator) {
		return strings.TrimSuffix(filepath.Base(value), l.Ext)
	}
	if Find(l, configDir, cwd, value) != "" {
		return value
	}
	if id := Latest(l, configDir, cwd, named); id != "" {
		return id
	}
	return value
}

// Latest is the id of the session `--continue` resumes: the most recently written
// transcript of the project of cwd (the current directory's, not another's) that
// accept lets through, or "" when there is none.
func Latest(l transcript.Layout, configDir, cwd string, accept func(path string) bool) string {
	dir := l.ProjectDir(configDir, cwd)
	entries, _ := os.ReadDir(dir)
	best, bestTime := "", int64(0)
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || !strings.HasSuffix(e.Name(), l.Ext) || !accept(filepath.Join(dir, e.Name())) {
			continue
		}
		if t := info.ModTime().UnixNano(); best == "" || t > bestTime {
			best, bestTime = strings.TrimSuffix(e.Name(), l.Ext), t
		}
	}
	return best
}
