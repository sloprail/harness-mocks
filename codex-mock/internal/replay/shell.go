package replay

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// words are the words of a command line, unquoted, when it is one simple
// command; nil when it is anything else (a pipeline, a list, a redirect).
func words(line string) []string {
	f, err := syntax.NewParser().Parse(strings.NewReader(line), "")
	if err != nil || len(f.Stmts) != 1 {
		return nil
	}
	st := f.Stmts[0]
	call, ok := st.Cmd.(*syntax.CallExpr)
	if !ok || st.Background || st.Negated || len(st.Redirs) > 0 || len(call.Assigns) > 0 {
		return nil
	}
	out := make([]string, len(call.Args))
	for i, w := range call.Args {
		f, err := expand.Fields(&expand.Config{}, w)
		if err != nil || len(f) != 1 {
			return nil
		}
		out[i] = f[0]
	}
	return out
}

// shellWords reports whether a command line is a shell's invocation of a
// command (/bin/<shell> -c <command>, or -lc), and the command it runs.
// The words after -c are the command, joined: that is how a process listing
// shows one whose quotes are gone.
func shellWords(w []string) (inner string, ok bool) {
	if len(w) < 3 || !strings.HasPrefix(w[0], "/bin/") || strings.Contains(w[0][len("/bin/"):], "/") {
		return "", false
	}
	if w[1] != "-c" && w[1] != "-lc" {
		return "", false
	}
	return strings.Join(w[2:], " "), true
}

// shellInner is the command inside `/bin/<shell> -c[l] <command>`, however it
// was quoted (double quotes as codex writes it, single quotes as the mock does,
// none for a single word); a line that is not such an invocation is returned
// as it is.
func shellInner(line string) string {
	w := words(line)
	if len(w) != 3 {
		return line
	}
	if inner, ok := shellWords(w); ok {
		return inner
	}
	return line
}

// isSleep reports whether the words are `sleep <seconds>`.
func isSleep(w []string) bool {
	return len(w) >= 2 && w[0] == "sleep" && regexp.MustCompile(`^\d+$`).MatchString(w[1])
}

// isJob reports whether a process's command line is the replayed job: a
// `sleep N`, or a shell running one.
func isJob(command string) bool {
	w := words(command)
	if isSleep(w) {
		return true
	}
	if inner, ok := shellWords(w); ok {
		return isSleep(words(inner))
	}
	return false
}

// jobLabel is a job's command line with the shell's path as <SHELL>.
func jobLabel(command string) string {
	w := words(command)
	if _, ok := shellWords(w); ok {
		w[0] = "<SHELL>"
	}
	return strings.Join(w, " ")
}
