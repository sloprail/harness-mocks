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

// checkPackage: when any code of the package (a function, a variable initializer, TestMain) names
// os/exec.LookPath, the package looks a tool up, and every Skip, Skipf and SkipNow of the testing package in it
// (a call, a method value, a generic or interface method) is refused unless it is the opt-in gate.
func checkPackage(fset *token.FileSet, imp types.Importer, name string, files []*ast.File) ([]string, error) {
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: imp, Error: func(error) {}, FakeImportC: true}
	if pkg, _ := conf.Check(name, fset, files, info); pkg == nil {
		return nil, fmt.Errorf("type-checking %s produced no package", name)
	}
	var lookup token.Pos
	for id, obj := range info.Uses {
		if fn, ok := obj.(*types.Func); ok && isLookPath(fn) && (lookup == token.NoPos || id.Pos() < lookup) {
			lookup = id.Pos()
		}
	}
	if lookup == token.NoPos {
		return nil, nil
	}
	var out []string
	for _, f := range files {
		walk(f, func(n ast.Node, stack []ast.Node) {
			id, ok := n.(*ast.Ident)
			if !ok {
				return
			}
			fn, ok := info.Uses[id].(*types.Func)
			if !ok || !isSkip(fn) || exempt(id, stack, info) {
				return
			}
			out = append(out, fmt.Sprintf("%s: (*testing.T).%s is used in package %s, which reaches os/exec.LookPath (%s): a test whose required tool is missing fails, it never skips", fset.Position(id.Pos()), fn.Name(), name, fset.Position(lookup)))
		})
	}
	return out, nil
}

func isLookPath(fn *types.Func) bool {
	return fn.Pkg() != nil && fn.Pkg().Path() == "os/exec" && fn.Name() == "LookPath"
}

// isSkip: a Skip, Skipf or SkipNow method of the testing package (T, B, F, TB and a generic instance of them).
func isSkip(fn *types.Func) bool {
	fn = fn.Origin()
	if fn.Pkg() == nil || fn.Pkg().Path() != "testing" {
		return false
	}
	switch fn.Name() {
	case "Skip", "Skipf", "SkipNow":
		return fn.Type().(*types.Signature).Recv() != nil
	}
	return false
}

func walk(root ast.Node, visit func(n ast.Node, stack []ast.Node)) {
	var stack []ast.Node
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		stack = append(stack, n)
		visit(n, stack)
		return true
	})
}
