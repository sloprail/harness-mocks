package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// checkDir judges the Go packages of dir. A directory that does not exist, or holds no .go file, is an
// error (the checker could not look), never an empty pass.
func checkDir(fset *token.FileSet, imp types.Importer, dir string) ([]string, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%s holds no .go file", dir)
	}
	sort.Strings(names)
	groups := map[string]map[string][]*ast.File{} // by the package name with _test stripped
	var out []string
	for _, n := range names {
		f, perr := parser.ParseFile(fset, n, nil, 0)
		if perr != nil {
			out = append(out, fmt.Sprintf("%s does not parse: %v", n, perr))
			continue
		}
		base := strings.TrimSuffix(f.Name.Name, "_test")
		if groups[base] == nil {
			groups[base] = map[string][]*ast.File{}
		}
		groups[base][f.Name.Name] = append(groups[base][f.Name.Name], f)
	}
	var pkgs []string
	for p := range groups {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		v, err := checkPackage(fset, imp, p, groups[p])
		if err != nil {
			return nil, err
		}
		out = append(out, v...)
	}
	return out, nil
}

// pkgUnit is one type-checked package of a directory: foo, or its external test package foo_test.
type pkgUnit struct {
	name  string
	files []*ast.File
	info  *types.Info
}

// checkPackage judges foo and foo_test together (the groups of checkDir are keyed by the name with _test
// stripped, so a skip moved to the external test package cannot escape): when any code of either (a function, a
// variable initializer, TestMain) names os/exec.LookPath, the package looks a tool up, and then
// every Skip, Skipf and SkipNow (a call, a method value, a generic, interface or type-parameter method,
// wherever the interface is declared) is refused unless it is the opt-in gate.
//
// Only a lookup inside the test package counts, not one through another package, and only a named
// os/exec.LookPath counts (an aliased or dot-imported one too); exec.Command is not treated as a lookup.
func checkPackage(fset *token.FileSet, imp types.Importer, base string, parts map[string][]*ast.File) ([]string, error) {
	var names []string
	for n := range parts {
		names = append(names, n)
	}
	sort.Strings(names)
	var units []pkgUnit
	for _, n := range names {
		info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
		conf := types.Config{Importer: imp, Error: func(error) {}, FakeImportC: true}
		if pkg, _ := conf.Check(n, fset, parts[n], info); pkg == nil {
			return nil, fmt.Errorf("type-checking %s produced no package", n)
		}
		units = append(units, pkgUnit{n, parts[n], info})
	}
	var lookup token.Pos
	for _, u := range units {
		for id, obj := range u.info.Uses {
			if fn, ok := obj.(*types.Func); ok && isLookPath(fn) && (lookup == token.NoPos || id.Pos() < lookup) {
				lookup = id.Pos()
			}
		}
	}
	if lookup == token.NoPos {
		return nil, nil
	}
	var out []string
	for _, u := range units {
		for _, f := range u.files {
			walk(f, func(n ast.Node, stack []ast.Node) {
				id, ok := n.(*ast.Ident)
				if !ok {
					return
				}
				fn, ok := u.info.Uses[id].(*types.Func)
				if !ok {
					return
				}
				switch {
				case isSkip(fn) && !exempt(id, stack, u.info):
					out = append(out, fmt.Sprintf("%s: %s is used in package %s, which reaches os/exec.LookPath (%s): a test whose required tool is missing fails, it never skips", fset.Position(id.Pos()), fn.Name(), base, fset.Position(lookup)))
				}
			})
		}
	}
	return out, nil
}
