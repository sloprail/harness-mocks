package toolexec

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// UnmodeledSyntax says which form of a shell command line no recording shows
// Cursor's frames for, "" when every form in it is one the mock models. What is
// modeled is what runs/shell-exit-status, runs/shell-syntax and the other
// recordings show: simple commands, pipes and `;`, single- and double-quoted
// words, numbers, a command substitution, and the redirections > >> < and 2>&1. The
// rest (variables, `&&` and `||`, here-documents, globs, subshells, loops,
// assignments, background jobs) is refused rather than reported with a guessed
// parse, so a run cannot pass on frames the real harness was never seen to write.
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
		case *syntax.ParamExp:
			return flag("a variable or parameter expansion")
		case *syntax.ArithmExp, *syntax.ArithmCmd:
			return flag("arithmetic")
		case *syntax.ProcSubst:
			return flag("a process substitution")
		case *syntax.Subshell, *syntax.Block:
			return flag("a subshell or a block")
		case *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause, *syntax.CaseClause, *syntax.FuncDecl:
			return flag("a loop, a conditional or a function")
		case *syntax.BinaryCmd:
			if x.Op != syntax.Pipe && x.Op != syntax.PipeAll {
				return flag("`" + x.Op.String() + "`")
			}
		case *syntax.Stmt:
			if x.Background || x.Negated || x.Coprocess {
				return flag("a background job or a negation")
			}
			for _, r := range x.Redirs {
				switch r.Op {
				case syntax.RdrOut, syntax.AppOut, syntax.RdrIn, syntax.DplOut:
				default:
					return flag("the redirection " + r.Op.String())
				}
			}
		case *syntax.CallExpr:
			if len(x.Assigns) > 0 {
				return flag("an assignment before a command")
			}
		case *syntax.CmdSubst:
			if x.Backquotes {
				return flag("a command substitution in backquotes")
			}
		case *syntax.Word: // an unquoted piece of a word: quoted text is never a glob
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
