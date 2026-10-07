package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"sort"
	"strconv"
	"strings"
)

const (
	listFile = "replay_allowlist_test.go"
	genFile  = "generated_replay_test.go"
	listName = "notReplaying"
)

// rankOf is how settled a reason is: 0 flaky:, 1 untriaged:, 2 any other reason.
func rankOf(reason string) int {
	switch {
	case strings.HasPrefix(reason, "flaky:"):
		return 0
	case strings.HasPrefix(reason, "untriaged:"):
		return 1
	}
	return 2
}

// knownReason is whether a reason starts with one of the categories the user named.
func knownReason(reason string) int {
	for _, p := range []string{"adapter:", "mock gap:", "untriaged:", "flaky:"} {
		if strings.HasPrefix(reason, p) {
			return 1
		}
	}
	return 0
}

// entriesOf reads the notReplaying map of a list file: declared exactly once, as a map[string]string
// composite literal whose keys and reasons are string literals (decoded, so an escape or a raw string
// cannot hide a category), and named nowhere else in the file.
func entriesOf(r io.Reader) (lines []string, violations []string, err error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	f, perr := parser.ParseFile(fset, listFile, src, 0)
	if perr != nil {
		return nil, []string{listFile + " does not parse: " + perr.Error()}, nil
	}
	var specs []*ast.ValueSpec
	var vals []ast.Expr
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if n.Name == listName {
					specs = append(specs, vs)
					if i < len(vs.Values) {
						vals = append(vals, vs.Values[i])
					} else {
						vals = append(vals, nil)
					}
				}
			}
		}
	}
	if len(specs) != 1 {
		return nil, []string{fmt.Sprintf("%s: %s must be declared exactly once at package level, found %d declarations", listFile, listName, len(specs))}, nil
	}
	uses := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == listName {
			uses++
		}
		return true
	})
	if uses != 1 {
		return nil, []string{fmt.Sprintf("%s: %s is named %d times in the file, but only its declaration may name it", listFile, listName, uses)}, nil
	}
	lit, ok := vals[0].(*ast.CompositeLit)
	if !ok {
		return nil, []string{fmt.Sprintf("%s: %s must be a map composite literal", listFile, listName)}, nil
	}
	mt, ok := lit.Type.(*ast.MapType)
	if !ok || !isIdent(mt.Key, "string") || !isIdent(mt.Value, "string") {
		return nil, []string{fmt.Sprintf("%s: %s must be a map[string]string literal", listFile, listName)}, nil
	}
	seen := map[string]bool{}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: an entry of %s is not a key: value pair", listFile, listName)}, nil
		}
		key, kok := stringLit(kv.Key)
		reason, rok := stringLit(kv.Value)
		if !kok || !rok {
			return nil, []string{fmt.Sprintf("%s: an entry of %s is not a pair of string literals (line %d)", listFile, listName, fset.Position(kv.Pos()).Line)}, nil
		}
		if strings.ContainsAny(key, "\t\n") {
			return nil, []string{fmt.Sprintf("%s: a key of %s has a tab or a newline", listFile, listName)}, nil
		}
		if seen[key] {
			return nil, []string{fmt.Sprintf("%s: %s lists %q twice", listFile, listName, key)}, nil
		}
		seen[key] = true
		lines = append(lines, fmt.Sprintf("%s\t%d\t%d", key, rankOf(reason), knownReason(reason)))
	}
	sort.Strings(lines)
	return lines, nil, nil
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// stringLit decodes a string literal (interpreted or raw); anything else is not one.
func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}
