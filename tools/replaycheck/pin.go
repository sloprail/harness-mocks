package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	testFunc = "TestGeneratedReplay"
	// the canonical copies, in the rule's own folder: replayUntilGreen once, the test once per mock
	untilCanon = "replay_until_green.go.txt"
	testCanon  = "test_generated_replay.*.go.txt"
)

// printFunc prints a function declaration as gofmt would, without comments (the files are parsed
// without them), so two copies of the same code are the same bytes whatever they were commented or
// spaced like.
func printFunc(fset *token.FileSet, fn *ast.FuncDecl) (string, error) {
	var b bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&b, fset, fn); err != nil {
		return "", err
	}
	return squeeze(b.String()), nil
}

// squeeze drops the blank lines (the printer keeps the ones a comment's lines left) and the edges.
func squeeze(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n") + "\n"
}

// funcsNamed are the package-level (non-method) function declarations called name, over files.
func funcsNamed(files []*ast.File, name string) []*ast.FuncDecl {
	var out []*ast.FuncDecl
	for _, f := range files {
		for _, d := range f.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == name {
				out = append(out, fd)
			}
		}
	}
	return out
}

// canonical reads the canonical copies matching pattern in dir.
func canonical(dir, pattern string) ([]string, error) {
	names, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var out []string
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			return nil, err
		}
		out = append(out, squeeze(string(b)))
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no canonical copy %s in %s", pattern, dir)
	}
	return out, nil
}

// checkPinned: replayUntilGreen and TestGeneratedReplay are, once printed, byte for byte the canonical
// copies of the rule's folder. What the failure of a never-green flaky: entry and the repetition depend on
// is these two functions, so they are pinned instead of analysed: a change to them is a change to the rule.
func (c *pkgCheck) checkPinned() error {
	for _, p := range []struct{ name, pattern string }{{untilGreen, untilCanon}, {testFunc, testCanon}} {
		fns := funcsNamed(c.files, p.name)
		if len(fns) != 1 {
			c.add(c.gen.Pos(), "%s must be defined exactly once in the package, found %d definitions", p.name, len(fns))
			continue
		}
		got, err := printFunc(c.fset, fns[0])
		if err != nil {
			return err
		}
		want, err := canonical(c.canon, p.pattern)
		if err != nil {
			return err
		}
		match := false
		for _, w := range want {
			match = match || w == got
		}
		if !match {
			c.add(fns[0].Pos(), "%s differs from the canonical copy in the rule's folder (%s/%s): it is pinned so that a never-green flaky: entry is run %s times and fails; changing it is a change to the rule and needs the user's citation", p.name, "canonical", p.pattern, flakyRunsName)
		}
	}
	return nil
}

// printNamed writes the function name of the Go file, as checkPinned compares it.
func printNamed(file, name string, w *os.File) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return err
	}
	fns := funcsNamed([]*ast.File{f}, name)
	if len(fns) != 1 {
		return fmt.Errorf("%s: found %d functions named %s", file, len(fns), name)
	}
	text, err := printFunc(fset, fns[0])
	if err != nil {
		return err
	}
	_, err = w.WriteString(text)
	return err
}
