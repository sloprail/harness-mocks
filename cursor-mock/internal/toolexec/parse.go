package toolexec

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// parsed is what Cursor reports of a shell command line before it runs it (the
// args of a shellToolCall frame, recorded: every Shell call of runs/*): its
// simple commands in order with their words, whether it redirects, and whether it
// substitutes commands. The command line is parsed by a shell parser, never
// picked apart by patterns.
type parsed struct {
	Failed     bool
	Names      []string
	Commands   []map[string]any
	Redirects  []map[string]any
	Substitute bool
}

// parseCommand parses a command line the way Cursor's frames show it.
func parseCommand(line string) parsed {
	f, err := syntax.NewParser().Parse(strings.NewReader(line), "")
	if err != nil {
		return parsed{Failed: true, Commands: []map[string]any{}, Redirects: []map[string]any{}, Names: []string{}}
	}
	p := parsed{Commands: []map[string]any{}, Redirects: []map[string]any{}, Names: []string{}}
	syntax.Walk(f, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.CmdSubst:
			p.Substitute = true
		case *syntax.Stmt:
			for _, r := range x.Redirs {
				p.Redirects = append(p.Redirects, redirect(line, r, stmtCommandName(line, x)))
			}
		case *syntax.CallExpr:
			if len(x.Args) == 0 || text(line, x.Args[0]) == "[" {
				break // `[ … ]` is a test, not a command Cursor lists (recorded: runs/shell-compound-test)
			}
			cmd := map[string]any{"name": text(line, x.Args[0]), "args": []any{},
				"fullText": line[x.Args[0].Pos().Offset():x.Args[len(x.Args)-1].End().Offset()]}
			args := []any{}
			for _, w := range x.Args[1:] {
				args = append(args, map[string]any{"type": wordType(w), "value": text(line, w)})
			}
			cmd["args"] = args
			p.Commands = append(p.Commands, cmd)
			p.Names = append(p.Names, text(line, x.Args[0]))
		}
		return true
	})
	return p
}

// stmtCommandName is the name of the command a statement runs: the target the frame
// gives a here-document (recorded: runs/shell-compound-forms, where `cat -n <<EOF` has "cat").
func stmtCommandName(line string, s *syntax.Stmt) string {
	if c, ok := s.Cmd.(*syntax.CallExpr); ok && len(c.Args) > 0 {
		return text(line, c.Args[0])
	}
	return ""
}

// text is the source text of a word, quotes included.
func text(line string, w *syntax.Word) string { return line[w.Pos().Offset():w.End().Offset()] }

var number = regexp.MustCompile(`^[0-9]+$`)

// wordType is what Cursor calls a word: a number, a single-quoted string (raw_string),
// a double-quoted one (string), a command substitution (command_substitution), or
// any other word (recorded: runs/shell-exit-status, runs/shell-syntax). An unquoted
// variable is a simple_expansion ($V, $1, $@), a braced one an expansion (${V}), and
// $(( )) an arithmetic_expansion (recorded: runs/shell-compound-forms,
// runs/shell-compound-more, runs/shell-compound-test). A word with a bracket glob is a
// concatenation (runs/shell-glob-tilde).
func wordType(w *syntax.Word) string {
	if bracketGlob(w) {
		return "concatenation" // `*`, `?` and `~` stay a word (recorded: runs/shell-glob-tilde)
	}
	if len(w.Parts) == 1 {
		switch p := w.Parts[0].(type) {
		case *syntax.ParamExp:
			if p.Short {
				return "simple_expansion"
			}
			return "expansion"
		case *syntax.ArithmExp:
			return "arithmetic_expansion"
		case *syntax.SglQuoted:
			return "raw_string"
		case *syntax.DblQuoted:
			return "string"
		case *syntax.CmdSubst:
			return "command_substitution"
		case *syntax.Lit:
			if number.MatchString(p.Value) {
				return "number"
			}
		}
	}
	return "word"
}

// bracketGlob is whether a word is plain text with a "[" in it (`[ab].txt`, `x[1]y`).
func bracketGlob(w *syntax.Word) bool {
	bracket := false
	for _, part := range w.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			return false
		}
		bracket = bracket || strings.Contains(lit.Value, "[")
	}
	return bracket
}
