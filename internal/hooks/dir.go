package hooks

import "os"

// FirstDir is the directory a command hook runs in: the first of candidates
// that is a directory that exists, so a hook whose working directory is gone (a
// worktree another shell deleted) falls back to the next. Empty candidates are
// skipped. When none exists it is the first candidate, which the hook then
// fails to start in, as it would have.
func FirstDir(candidates ...string) string {
	for _, d := range candidates {
		if st, err := os.Stat(d); d != "" && err == nil && st.IsDir() {
			return d
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}
