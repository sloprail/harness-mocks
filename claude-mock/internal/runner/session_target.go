package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// EnvSessionName carries --name to the session's transcript, which records it
// (the mock's own channel, like A10N_MOCK_SYSTEM_PROMPT).
const EnvSessionName = "A10N_MOCK_SESSION_NAME"

// ResumeTarget is the session id a `--resume` value names: the path of a session's
// transcript file (its name is the id), the id of a session of the directory's
// project, or the name a session was given with --name (claude 2.1.285 resumed
// each; recorded: snapshots/runs/resume-path, resume-name). A value that is none
// of them is returned as it is, for the run to fail on as an unknown session.
//
// sr:provides session-resume/claude
func ResumeTarget(configDir, cwd, value string) string {
	if strings.HasSuffix(value, claudeLayout.Ext) || strings.ContainsRune(value, filepath.Separator) {
		return strings.TrimSuffix(filepath.Base(value), claudeLayout.Ext)
	}
	if _, err := os.Stat(sessionFilePath(resolveConfigDir(configDir), cwd, value)); err == nil {
		return value
	}
	if id := sessionNamed(configDir, cwd, value); id != "" {
		return id
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
	return newestSession(configDir, cwd, func(string) bool { return true })
}

// sessionNamed is the id of the project's most recently written session that was
// given name, by the agent-name record that --name leaves.
func sessionNamed(configDir, cwd, name string) string {
	return newestSession(configDir, cwd, func(path string) bool {
		data, err := os.ReadFile(path)
		return err == nil && transcriptAgentName(data) == name
	})
}

// transcriptAgentName is the last agent-name a transcript records, or "".
func transcriptAgentName(data []byte) string {
	name := ""
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, `"agent-name"`) {
			continue
		}
		var rec struct {
			Type      string `json:"type"`
			AgentName string `json:"agentName"`
		}
		if json.Unmarshal([]byte(line), &rec) == nil && rec.Type == "agent-name" {
			name = rec.AgentName
		}
	}
	return name
}

// newestSession is the id of the most recently written transcript of the
// directory's project that match accepts, or "".
func newestSession(configDir, cwd string, match func(path string) bool) string {
	dir := claudeLayout.ProjectDir(resolveConfigDir(configDir), resolveEncodingCwd(cwd))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestTime int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() || !strings.HasSuffix(e.Name(), claudeLayout.Ext) || !match(filepath.Join(dir, e.Name())) {
			continue
		}
		if t := info.ModTime().UnixNano(); best == "" || t > bestTime {
			best, bestTime = strings.TrimSuffix(e.Name(), claudeLayout.Ext), t
		}
	}
	return best
}
