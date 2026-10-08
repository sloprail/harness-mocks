package toolexec

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// UnmodeledSyntax says which form of a shell command line no recording shows
// Cursor's frames for, "" when every form in it is one the mock models. What is
// modeled is what runs/shell-exit-status, runs/shell-syntax, runs/shell-compound,
// runs/shell-compound-forms, runs/shell-compound-more and runs/shell-compound-test
// show: the command line runs in a shell, so any compound form runs, and the frame
// lists the simple commands in it in source order (`&&`, `||`, loops, conditionals,
// functions, subshells and blocks, background jobs, negation and assignments before
// a command included), with variables and $(( )), here-documents and here-strings
// besides the words, pipes, command substitution and redirections > >> < >&. The
// rest (process substitution, backquotes, (( )), a coprocess, other redirections, a
// here-document on anything but a command) is refused rather than reported with a
// guessed parse, so a run cannot pass on frames the real harness was never seen to write. (A glob or a `~` in a word is modeled: it
// is expanded by the shell, and a glob matching nothing fails the command; recorded:
// runs/shell-glob-tilde.)
func UnmodeledSyntax(line string) string {
	f, err := syntax.NewParser().Parse(strings.NewReader(line), "")
	if err != nil {
		return "" // a line that does not parse is reported as a failed parse
	}
	why := ""
	flag := func(s string) bool {
		if why == "" {
			why = s
		}
		return false
	}
	syntax.Walk(f, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ArithmCmd:
			return flag("arithmetic as a command")
		case *syntax.ProcSubst:
			return flag("a process substitution")
		case *syntax.Stmt:
			if x.Coprocess {
				return flag("a coprocess")
			}
			for _, r := range x.Redirs {
				switch r.Op {
				case syntax.RdrOut, syntax.AppOut, syntax.RdrIn, syntax.DplOut, syntax.WordHdoc:
				case syntax.Hdoc, syntax.DashHdoc:
					if c, ok := x.Cmd.(*syntax.CallExpr); !ok || len(c.Args) == 0 {
						return flag("a here-document on something other than a command")
					}
				default:
					return flag("the redirection " + r.Op.String())
				}
			}
		case *syntax.CmdSubst:
			if x.Backquotes {
				return flag("a command substitution in backquotes")
			}
		}
		return true
	})
	return why
}
