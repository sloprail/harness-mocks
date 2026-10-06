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
				p.Redirects = append(p.Redirects, redirect(line, r))
			}
		case *syntax.CallExpr:
			if len(x.Args) == 0 {
				break
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

// text is the source text of a word, quotes included.
func text(line string, w *syntax.Word) string { return line[w.Pos().Offset():w.End().Offset()] }

var number = regexp.MustCompile(`^[0-9]+$`)

// wordType is what Cursor calls a word: a number, a single-quoted string (raw_string),
// a double-quoted one (string), or any other word (recorded: word, number and
// raw_string; a double-quoted word is not in any recording, "string" is the name
// the parser gives it).
func wordType(w *syntax.Word) string {
	if len(w.Parts) == 1 {
		switch p := w.Parts[0].(type) {
		case *syntax.SglQuoted:
			return "raw_string"
		case *syntax.DblQuoted:
			return "string"
		case *syntax.Lit:
			if number.MatchString(p.Value) {
				return "number"
			}
		}
	}
	return "word"
}

// redirect is one redirection as the frame lists it: its operator, the file
// descriptors it writes to or reads, and its target (recorded: a `>` of a word).
func redirect(line string, r *syntax.Redirect) map[string]any {
	fd := 1
	switch r.Op {
	case syntax.RdrIn, syntax.RdrInOut, syntax.DplIn, syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
		fd = 0
	}
	if r.N != nil {
		fd = 0
		for _, c := range r.N.Value {
			fd = fd*10 + int(c-'0')
		}
	}
	target, kind := "", "word"
	if r.Word != nil {
		target = text(line, r.Word)
		kind = wordType(r.Word)
		if kind == "number" || kind == "raw_string" {
			kind = "word"
		}
	}
	return map[string]any{"operator": r.Op.String(), "destinationFds": []any{fd}, "targetNodeType": kind, "targetText": target}
}

// outputRedirects reports whether any redirection writes, and devNull whether all
// of them go to /dev/null.
func (p parsed) outputRedirects() (writes, devNull bool) {
	devNull = true
	for _, r := range p.Redirects {
		if fds, _ := r["destinationFds"].([]any); len(fds) > 0 && fds[0] != 0 {
			writes = true
		}
		if r["targetText"] != "/dev/null" {
			devNull = false
		}
	}
	return writes, devNull
}

// inputRedirects reports whether any redirection reads.
func (p parsed) inputRedirects() bool {
	for _, r := range p.Redirects {
		if fds, _ := r["destinationFds"].([]any); len(fds) > 0 && fds[0] == 0 {
			return true
		}
	}
	return false
}
