package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
)

// unit is one function declaration (its literals included): what it uses.
type unit struct {
	fd      *ast.FuncDecl
	obj     *types.Func
	refs    map[*types.Func]bool // the functions it names (package functions, LookPath, Skip methods)
	lookup  bool                 // reaches os/exec.LookPath
	skipper bool                 // uses a testing Skip, or names a skipper
}

func checkDir(fset *token.FileSet, imp types.Importer, dir string) ([]string, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	groups := map[string][]*ast.File{}
	var out []string
	for _, n := range names {
		f, perr := parser.ParseFile(fset, n, nil, 0)
		if perr != nil {
			out = append(out, fmt.Sprintf("%s does not parse: %v", n, perr))
			continue
		}
		groups[f.Name.Name] = append(groups[f.Name.Name], f)
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

func checkPackage(fset *token.FileSet, imp types.Importer, name string, files []*ast.File) ([]string, error) {
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: imp, Error: func(error) {}, FakeImportC: true}
	if pkg, _ := conf.Check(name, fset, files, info); pkg == nil {
		return nil, fmt.Errorf("type-checking %s produced no package", name)
	}
	units := collect(files, info)
	propagate(units)
	var out []string
	for _, u := range units {
		if !u.lookup {
			continue
		}
		walk(u.fd.Body, func(n ast.Node, stack []ast.Node) {
			id, ok := n.(*ast.Ident)
			if !ok {
				return
			}
			fn, ok := info.Uses[id].(*types.Func)
			if !ok || !(isSkip(fn) || (units[fn] != nil && units[fn].skipper)) || exempt(id, stack, info) {
				return
			}
			what := "(*testing.T)." + fn.Name()
			if !isSkip(fn) {
				what = "the helper " + fn.Name() + ", which skips,"
			}
			out = append(out, fmt.Sprintf("%s: %s is used in %s, which reaches os/exec.LookPath: a test whose required tool is missing fails, it never skips", fset.Position(id.Pos()), what, u.fd.Name.Name))
		})
	}
	return out, nil
}
