package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strings"
)

// item is one comparable unit of a package: a whole top-level declaration
// with its doc comment, or a comment group attached to no declaration.
type item struct {
	Kind string // "decl" or "comment"
	File string
	Text string
	Sig  string // package name and build sensitivity of the file it sits in
	Pin  bool   // must stay in the same file (init funcs, //go: comments)
}

func (i item) key() string {
	k := i.Kind + "|" + i.Sig + "|"
	if i.Pin {
		k += "@" + i.File
	}
	return k + "\n" + i.Text
}

func (i item) String() string {
	return "[" + path.Base(i.File) + "] " + i.Kind + " " + strings.TrimSpace(i.Text)
}

// parseItems returns the items of one file and its package name. Package
// clauses and import declarations are left out on purpose: a move may edit
// them.
func parseItems(name string, src []byte) ([]item, string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, "", err
	}
	sig := fileSig(name, f)
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	type span struct{ lo, hi int }
	var spans []span
	var items []item
	for _, d := range f.Decls {
		lo, hi := off(d.Pos()), off(d.End())
		var doc *ast.CommentGroup
		switch d := d.(type) {
		case *ast.FuncDecl:
			doc = d.Doc
		case *ast.GenDecl:
			doc = d.Doc
		}
		if doc != nil {
			lo = off(doc.Pos())
		}
		for _, g := range f.Comments { // trailing comment on the last line
			if off(g.Pos()) >= hi && fset.Position(g.Pos()).Line == fset.Position(d.End()).Line {
				hi = off(g.End())
			}
		}
		spans = append(spans, span{lo, hi})
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
			continue
		}
		fd, isFunc := d.(*ast.FuncDecl)
		items = append(items, item{Kind: "decl", File: name, Text: string(src[lo:hi]), Sig: sig,
			Pin: isFunc && fd.Recv == nil && fd.Name.Name == "init"})
	}
	for _, g := range f.Comments {
		lo, hi := off(g.Pos()), off(g.End())
		inside := false
		for _, s := range spans {
			inside = inside || (lo >= s.lo && hi <= s.hi)
		}
		if !inside {
			text := string(src[lo:hi])
			items = append(items, item{Kind: "comment", File: name, Text: text, Sig: sig,
				Pin: strings.HasPrefix(text, "//go:")})
		}
	}
	return items, f.Name.Name, nil
}

func sortItems(items []item) {
	sort.Slice(items, func(a, b int) bool { return items[a].key() < items[b].key() })
}
