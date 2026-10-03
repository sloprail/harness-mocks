package runner

import (
	"os"
	"path/filepath"
	"strings"
)

// ResumeTarget is the session id a `--resume` value names: an id as it is, or
// the path of a session's transcript file, whose name is the id (claude 2.1.285
// resumed a session by `--resume <path>.jsonl`, appended to that file and told
// its hooks the session's own id; recorded: snapshots/runs/resume-path).
//
// sr:provides session-resume/claude
func ResumeTarget(value string) string {
	if strings.HasSuffix(value, claudeLayout.Ext) || strings.ContainsRune(value, filepath.Separator) {
		return strings.TrimSuffix(filepath.Base(value), claudeLayout.Ext)
	}
	return value
}

// LatestSession is the id of the session `--continue` resumes: the most recently
// written transcript of the directory's project, or "" when it has none
// (recorded: snapshots/runs/resume-continue, which continued the directory's one
// session in its own file, SessionStart source "resume").
//
// sr:provides session-resume/claude
func LatestSession(configDir, cwd string) string {
	dir := claudeLayout.ProjectDir(resolveConfigDir(configDir), resolveEncodingCwd(cwd))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestTime int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || !strings.HasSuffix(e.Name(), claudeLayout.Ext) {
			continue
		}
		if t := info.ModTime().UnixNano(); best == "" || t > bestTime {
			best, bestTime = strings.TrimSuffix(e.Name(), claudeLayout.Ext), t
		}
	}
	return best
}
