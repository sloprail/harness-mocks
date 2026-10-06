package replay

import "encoding/json"

// mainMockArgs are the words the first run gives the mock: its own session flags, as given (a name
// or a path stays what it was), and the flags the mock models.
func mainMockArgs(s stepSpec) []string {
	var out []string
	switch {
	case s.resume != "":
		out = append(out, "--resume", s.resume)
	case s.cont:
		out = append(out, "--continue")
	}
	if s.newID != "" {
		out = append(out, "--session-id", s.newID)
	}
	if s.fork {
		out = append(out, "--fork-session")
	}
	return append(out, s.args...)
}

// stepMockArgs are the words a later run gives the mock: its session flags (the first session is
// the replay's own, <SESSION>) and the flags the mock models.
func stepMockArgs(s stepSpec) []string {
	var out []string
	switch {
	case s.resume != "":
		out = append(out, "--resume", "<SESSION>")
	case s.cont:
		out = append(out, "--continue")
	case s.newID != "":
		out = append(out, "--session-id", s.newID)
	}
	if s.fork {
		out = append(out, "--fork-session")
	}
	if (s.resume != "" || s.cont) && s.newID != "" {
		out = append(out, "--session-id", s.newID)
	}
	return append(out, s.args...)
}

// stepFiles are what a later run's directory brings beside its words: the symlink made before it and the
// project files of the directory it runs in.
type stepFiles struct{ Symlink, Settings, Hook string }

func stepFilesJSON(specs []stepSpec) string {
	files := make([]stepFiles, len(specs))
	any := false
	for i, s := range specs {
		files[i] = stepFiles{s.symlink, s.settings, s.hook}
		any = any || s.symlink != "" || s.settings != "" || s.hook != ""
	}
	if !any {
		return ""
	}
	b, _ := json.Marshal(files)
	return string(b)
}
