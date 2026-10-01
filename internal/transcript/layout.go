// Package transcript is the harness-neutral core of a session's transcript: the
// file it lives in, its creation, and the bookkeeping on its records.
package transcript

import (
	"os"
	"path/filepath"
)

// Layout is how a harness keys its transcript files under its configuration
// directory: a projects directory, one subdirectory per working directory, one
// file per session.
type Layout struct {
	// ProjectsDir is the directory under the configuration directory that holds
	// one subdirectory per working directory.
	ProjectsDir string
	// Encode is the subdirectory name of a (symlink-resolved) working directory.
	Encode func(dir string) string
	// Ext is the transcript file's extension, dot included.
	Ext string
}

// ResolveDir is dir with its symlinks resolved, or dir itself when it cannot be
// (a directory that does not exist yet): the form a harness keys a session by
// and reports in its payloads.
func ResolveDir(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}

// ProjectDir is the directory holding the transcripts of sessions run in cwd.
func (l Layout) ProjectDir(configDir, cwd string) string {
	return filepath.Join(configDir, l.ProjectsDir, l.Encode(ResolveDir(cwd)))
}

// FilePath is the transcript of session sessionID run in cwd: under the
// configuration directory, keyed by cwd with its symlinks resolved and by the
// session id.
//
// sr:capability session-transcript-file
func (l Layout) FilePath(configDir, cwd, sessionID string) string {
	return filepath.Join(l.ProjectDir(configDir, cwd), sessionID+l.Ext)
}

// Lazy is a transcript file that does not exist until the first record is
// written: a fresh session's file is absent while the session's start hook
// runs. It also separates the path records go to from the path a harness
// reports for it, which differ for a session resumed from another directory.
type Lazy struct {
	// Path is where records are written.
	Path string
	// Reported is what hook payloads carry as the transcript's path.
	Reported string

	f     *os.File
	fresh bool
}

// NewLazy is a handle on path. The file is opened now when it exists already
// (a resume, a caller that seeded it) and on the first write otherwise.
// reported defaults to path.
func NewLazy(path, reported string) (*Lazy, error) {
	l := &Lazy{Path: path, Reported: reported}
	if l.Reported == "" {
		l.Reported = path
	}
	if _, err := os.Stat(path); err == nil {
		if err := l.open(); err != nil {
			return nil, err
		}
	} else {
		l.fresh = true
	}
	return l, nil
}

func (l *Lazy) open() error {
	if l.f != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.f = f
	return nil
}

// Exists reports whether the file is on disk.
func (l *Lazy) Exists() bool {
	_, err := os.Stat(l.Path)
	return err == nil
}

// File is the open file, created now if nothing has been written yet. When this
// call creates it, head (if not nil) returns the lines it opens with.
func (l *Lazy) File(head func() [][]byte) *os.File {
	if l == nil {
		return nil
	}
	if l.f != nil {
		return l.f
	}
	if err := l.open(); err != nil {
		return nil
	}
	if l.fresh && head != nil {
		for _, line := range head() {
			l.f.Write(append(append([]byte{}, line...), '\n')) //nolint:errcheck
		}
	}
	return l.f
}

// Close closes the file if it was ever opened.
func (l *Lazy) Close() {
	if l != nil && l.f != nil {
		l.f.Close()
	}
}
