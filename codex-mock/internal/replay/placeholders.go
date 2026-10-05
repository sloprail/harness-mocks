package replay

import "strings"

// inRepo is a file the replay writes into the repository, with the run's directory where the
// scenario has the placeholder: a sub-agent's script holds calls, the run's own hook script is
// not ours to edit.
func inRepo(name, body, repo string) string {
	if name == "hook.sh" {
		return body
	}
	return strings.ReplaceAll(body, runPlaceholder, repo)
}

// scriptText is a scenario script with the directories its placeholders stand for.
func scriptText(body, scriptsAt, repo string) string {
	return strings.ReplaceAll(strings.ReplaceAll(body, scriptsDir, scriptsAt), runPlaceholder, repo)
}
