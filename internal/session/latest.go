package session

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sloprail/harness-mocks/internal/transcript"
)

// Latest is the id of the session a harness's `--continue` resumes: the most
// recently written transcript of the project of cwd (the current directory's,
// not another's) that accept lets through, or "" when there is none.
//
// sr:capability session-resume
func Latest(l transcript.Layout, configDir, cwd string, accept func(path string) bool) string {
	dir := l.ProjectDir(configDir, cwd)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
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

// IDOfPath is the session id a transcript file's path names (its file name
// without the layout's extension), and whether value is such a path.
//
// sr:capability session-resume
func IDOfPath(l transcript.Layout, value string) (string, bool) {
	if strings.HasSuffix(value, l.Ext) || strings.ContainsRune(value, filepath.Separator) {
		return strings.TrimSuffix(filepath.Base(value), l.Ext), true
	}
	return "", false
}
