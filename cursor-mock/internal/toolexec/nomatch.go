package toolexec

import (
	"sort"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// noMatchPrelude defines __nm LINE WORD…: the check Cursor's shell makes of an unquoted
// glob. WORD… is the glob as the shell expands it, so when nothing matches the first of
// them is the pattern itself (quotes removed).
const noMatchPrelude = `__nm() { [ -e "$2" ] || [ -L "$2" ] || { printf '(eval):%s: no matches found: %s\n' "$1" "$2" >&2; return 1; }; }; `

// withNoMatchCheck is the command line as Cursor's shell runs it. Cursor runs the user's
// shell (zsh), where an unquoted glob that matches no file is an error and its command does
// not run: the command ends with status 1 and "(eval):1: no matches found: <pattern>" on
// stderr, and the rest of the line carries on (`ls *.x || echo no`). A pattern that matches,
// a `~`, and a glob in quotes expand or stay as in bash, the shell the mock runs
// (recorded: runs/shell-glob-tilde). bash would run the command with the pattern as an
// argument, so each command with an unquoted glob word is wrapped in a block that checks the
// words first, when the line runs and from the directory it is then in.
func withNoMatchCheck(line string) string {
	f, err := syntax.NewParser().Parse(strings.NewReader(line), "")
	if err != nil {
		return line
	}
	type edit struct {
		at   int
		text string
	}
	var edits []edit
	syntax.Walk(f, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		var checks []string
		for i, w := range call.Args {
			if i > 0 && hasUnquotedGlob(w) {
				checks = append(checks, "__nm "+strconv.Itoa(int(w.Pos().Line()))+" "+line[w.Pos().Offset():w.End().Offset()])
			}
		}
		if len(checks) > 0 {
			edits = append(edits, edit{int(call.Pos().Offset()), "{ " + strings.Join(checks, " && ") + " && "}, edit{int(call.End().Offset()), "; }"})
		}
		return true
	})
	if len(edits) == 0 {
		return line
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].at > edits[j].at })
	for _, e := range edits {
		line = line[:e.at] + e.text + line[e.at:]
	}
	return noMatchPrelude + line
}

// hasUnquotedGlob is whether a word has a glob character outside quotes. A lone "[" or "]"
// is the test command's bracket (recorded: runs/shell-compound-test).
func hasUnquotedGlob(w *syntax.Word) bool {
	if l := w.Lit(); l == "[" || l == "]" {
		return false
	}
	for _, part := range w.Parts {
		if lit, ok := part.(*syntax.Lit); ok && strings.ContainsAny(lit.Value, "*?[") {
			return true
		}
	}
	return false
}
