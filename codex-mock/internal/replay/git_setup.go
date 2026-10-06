package replay

// gitSetup are the git commands that make the scratch repository a recording was made in: branch main,
// one empty commit, "init" (capture.sh; the host's default branch name is not behaviour). A run recorded
// outside a repository (setup/no-git) gets none.
func gitSetup(noGit bool) [][]string {
	if noGit {
		return nil
	}
	return [][]string{{"init", "-q", "-b", "main"},
		{"-c", "user.name=replay", "-c", "user.email=replay@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"}}
}
