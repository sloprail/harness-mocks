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
// glob or a home directory in a word, a here-document on anything but a command) is
// refused rather than reported with a guessed parse, so a run cannot pass on frames
// the real harness was never seen to write.
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
	listed := map[*syntax.Word]bool{} // the words a for loop runs over: they are not in the frame
	syntax.Walk(f, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ArithmCmd:
			return flag("arithmetic as a command")
		case *syntax.ProcSubst:
			return flag("a process substitution")
		case *syntax.ForClause:
			if w, ok := x.Loop.(*syntax.WordIter); ok {
				for _, item := range w.Items {
					listed[item] = true
				}
			}
		case *syntax.CaseClause: // the patterns of a case are matched by the shell, not listed in the frame (recorded: runs/shell-compound-more)
			for _, item := range x.Items {
				for _, pat := range item.Patterns {
					listed[pat] = true
				}
			}
		case *syntax.CallExpr: // `[ … ]` is a test: its brackets are not a glob (recorded: runs/shell-compound-test)
			if len(x.Args) > 1 && x.Args[0].Lit() == "[" && x.Args[len(x.Args)-1].Lit() == "]" {
				listed[x.Args[0]], listed[x.Args[len(x.Args)-1]] = true, true
			}
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
		case *syntax.Word: // an unquoted piece of a word: quoted text is never a glob
			if listed[x] {
				break
			}
			for _, part := range x.Parts {
				if lit, ok := part.(*syntax.Lit); ok && (strings.ContainsAny(lit.Value, "*?[") || strings.HasPrefix(lit.Value, "~")) {
					return flag("a glob or a home directory")
				}
			}
		}
		return true
	})
	return why
}
