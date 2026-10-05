package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	flakyRunsName = "flakyRuns"
	untilGreen    = "replayUntilGreen"
	caseWanted    = `flaky && (err != nil || diff != "")`
)

// fakeImporter answers every import with an empty package: the files are checked on their own, so
// what an import provides is not known, but every name the package itself declares resolves.
type fakeImporter struct{}

func (fakeImporter) Import(path string) (*types.Package, error) {
	name := path[strings.LastIndex(path, "/")+1:]
	p := types.NewPackage(path, strings.ReplaceAll(name, "-", "_"))
	p.MarkComplete()
	return p, nil
}

var _ types.Importer = fakeImporter{}

// checkDir judges the Go package files of dir:
//
//	(a) the list file declares notReplaying (see entriesOf)
//	(b) the generated test only reads it: an index expression that is not assigned, or a range
//	(c) flakyRuns is the package-level const 3 where replayUntilGreen is called, which is declared once
//	(d) no other file of the package names notReplaying
func checkDir(dir string) ([]string, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	fset := token.NewFileSet()
	groups := map[string][]*ast.File{}
	var violations []string
	for _, n := range names {
		src, err := os.ReadFile(n)
		if err != nil {
			return nil, err
		}
		f, perr := parser.ParseFile(fset, n, src, 0)
		if perr != nil {
			violations = append(violations, fmt.Sprintf("%s does not parse: %v", n, perr))
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
		v, err := checkPackage(fset, p, groups[p])
		if err != nil {
			return nil, err
		}
		violations = append(violations, v...)
	}
	return violations, nil
}

func base(fset *token.FileSet, f *ast.File) string {
	return filepath.Base(fset.Position(f.Pos()).Filename)
}

func checkPackage(fset *token.FileSet, pkgName string, files []*ast.File) ([]string, error) {
	var out []string
	add := func(pos token.Pos, format string, args ...any) {
		out = append(out, fmt.Sprintf("%s: %s", fset.Position(pos), fmt.Sprintf(format, args...)))
	}
	var list, gen *ast.File
	for _, f := range files {
		switch base(fset, f) {
		case listFile:
			list = f
		case genFile:
			gen = f
		}
	}
	// (d) the list is named only in its own two files (a shadowing or aliasing declaration included)
	for _, f := range files {
		if f == list || f == gen {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == listName {
				add(id.Pos(), "%s is named outside %s and %s, so an entry could be added there unseen", listName, listFile, genFile)
			}
			return true
		})
	}
	if list == nil && gen == nil {
		return out, nil
	}
	if list == nil {
		if mentions(gen, listName) {
			add(gen.Pos(), "%s reads %s, but %s is gone or renamed: keep the list in %s", genFile, listName, listFile, listFile)
		}
		return out, nil
	}
	if gen == nil {
		return out, nil
	}

	info := &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: fakeImporter{}, Error: func(error) {}, FakeImportC: true}
	pkg, _ := conf.Check(pkgName, fset, files, info)
	if pkg == nil {
		return nil, fmt.Errorf("type-checking %s produced no package", pkgName)
	}

	// (b) the generated test only reads the list
	listObj := pkg.Scope().Lookup(listName)
	if listObj == nil {
		add(list.Pos(), "%s is not declared at package level in %s", listName, listFile)
		return out, nil
	}
	walk(gen, func(n ast.Node, stack []ast.Node) {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != listName {
			return
		}
		if info.Uses[id] != listObj {
			add(id.Pos(), "%s here is not the package's list (a shadowing name): the generated test may only read the list itself", listName)
			return
		}
		if !isRead(id, stack) {
			add(id.Pos(), "the generated test may only read %s (a range, or an index that is not assigned), not change, alias or pass it", listName)
		}
	})

	// (c) flakyRuns and replayUntilGreen
	flakyObj, _ := pkg.Scope().Lookup(flakyRunsName).(*types.Const)
	if flakyObj == nil {
		add(gen.Pos(), "%s is not a package-level const", flakyRunsName)
	} else if v, ok := constant.Int64Val(constant.ToInt(flakyObj.Val())); !ok || v != 3 {
		add(flakyObj.Pos(), "%s must be 3, a flaky: entry is replayed three times", flakyRunsName)
	}
	if n := topLevelDecls(files, flakyRunsName); n != 1 {
		add(gen.Pos(), "%s must be declared exactly once at package level, found %d declarations", flakyRunsName, n)
	}
	var untilObj *types.Func
	if fo, ok := pkg.Scope().Lookup(untilGreen).(*types.Func); ok {
		untilObj = fo
	}
	if n := topLevelDecls(files, untilGreen); n != 1 || untilObj == nil {
		add(gen.Pos(), "%s must be defined exactly once at package level, found %d declarations", untilGreen, n)
	}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || id.Name != flakyRunsName {
				return true
			}
			if o := info.Defs[id]; o != nil && o != types.Object(flakyObj) {
				add(id.Pos(), "%s is declared again here: it must stay the one package-level const", flakyRunsName)
			}
			return true
		})
	}
	calls := 0
	walk(gen, func(n ast.Node, stack []ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}
		fun, ok := call.Fun.(*ast.Ident)
		if !ok || fun.Name != untilGreen {
			return
		}
		calls++
		if untilObj == nil || info.Uses[fun] != types.Object(untilObj) {
			add(fun.Pos(), "%s here is not the package's function", untilGreen)
		}
		if len(call.Args) != 2 {
			add(call.Pos(), "%s takes the replay and flakyRuns", untilGreen)
			return
		}
		arg, ok := call.Args[1].(*ast.Ident)
		if !ok || flakyObj == nil || info.Uses[arg] != types.Object(flakyObj) {
			add(call.Args[1].Pos(), "the second argument of %s must be the package-level const %s (not a shadowing name or another value)", untilGreen, flakyRunsName)
		}
		if as, ok := parent(stack).(*ast.AssignStmt); !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 || isBlank(as.Lhs[0]) && isBlank(as.Lhs[1]) {
			add(call.Pos(), "the result of %s must be kept (diff, err = ...) to be judged", untilGreen)
		}
	})
	if calls == 0 {
		add(gen.Pos(), "%s must run a flaky: entry through %s(run, %s)", genFile, untilGreen, flakyRunsName)
	}

	// a flaky: entry that is never green fails: the prefix is tested, and the case errors on it
	prefix, failCase := false, false
	ast.Inspect(gen, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "HasPrefix" && len(x.Args) == 2 {
				if s, ok := stringLit(x.Args[1]); ok && s == "flaky:" {
					prefix = true
				}
			}
		case *ast.CaseClause:
			for _, e := range x.List {
				if types.ExprString(e) == caseWanted {
					failCase = true
				}
			}
		}
		return true
	})
	if !prefix {
		add(gen.Pos(), "%s must tell a flaky: entry by strings.HasPrefix(reason, \"flaky:\")", genFile)
	}
	if !failCase {
		add(gen.Pos(), "%s must fail a flaky: entry that is never green: a case `%s` that errors", genFile, caseWanted)
	}
	return out, nil
}

func mentions(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return true
	})
	return found
}

// topLevelDecls counts the package-level declarations of name (const, var, type, func) over files.
func topLevelDecls(files []*ast.File, name string) int {
	n := 0
	for _, f := range files {
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				if x.Recv == nil && x.Name.Name == name {
					n++
				}
			case *ast.GenDecl:
				for _, s := range x.Specs {
					switch sp := s.(type) {
					case *ast.ValueSpec:
						for _, id := range sp.Names {
							if id.Name == name {
								n++
							}
						}
					case *ast.TypeSpec:
						if sp.Name.Name == name {
							n++
						}
					}
				}
			}
		}
	}
	return n
}

func isBlank(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "_"
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

// parent is the node above the last one of stack.
func parent(stack []ast.Node) ast.Node {
	if len(stack) < 2 {
		return nil
	}
	return stack[len(stack)-2]
}

// isRead: id (the last of stack) is the operand of a range, or of an index expression that is read
// (not assigned, incremented, address-taken, or the target of a range).
func isRead(id *ast.Ident, stack []ast.Node) bool {
	switch p := parent(stack).(type) {
	case *ast.RangeStmt:
		return p.X == ast.Expr(id)
	case *ast.IndexExpr:
		if p.X != ast.Expr(id) {
			return false
		}
		switch g := stack[len(stack)-3].(type) {
		case *ast.AssignStmt:
			for _, l := range g.Lhs {
				if l == ast.Expr(p) {
					return false
				}
			}
		case *ast.IncDecStmt:
			return false
		case *ast.UnaryExpr:
			return false
		case *ast.RangeStmt:
			return g.Key != ast.Expr(p) && g.Value != ast.Expr(p)
		}
		return true
	}
	return false
}
