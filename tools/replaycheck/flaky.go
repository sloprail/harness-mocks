package main

import (
	"go/ast"
	"go/constant"
	"go/types"
)

const (
	flakyRunsName = "flakyRuns"
	untilGreen    = "replayUntilGreen"
	caseWanted    = `flaky && (err != nil || diff != "")`
)

// checkFlaky is (c): flakyRuns is the one package-level const 3, replayUntilGreen is defined once, and
// the generated test runs a flaky: entry through replayUntilGreen(run, flakyRuns) with that very const,
// keeps the result, and fails an entry that is never green.
func (c *pkgCheck) checkFlaky() {
	flakyObj, _ := c.pkg.Scope().Lookup(flakyRunsName).(*types.Const)
	if flakyObj == nil {
		c.add(c.gen.Pos(), "%s is not a package-level const", flakyRunsName)
	} else if v, ok := constant.Int64Val(constant.ToInt(flakyObj.Val())); !ok || v != 3 {
		c.add(flakyObj.Pos(), "%s must be 3, a flaky: entry is replayed three times", flakyRunsName)
	}
	if n := topLevelDecls(c.files, flakyRunsName); n != 1 {
		c.add(c.gen.Pos(), "%s must be declared exactly once at package level, found %d declarations", flakyRunsName, n)
	}
	untilObj, _ := c.pkg.Scope().Lookup(untilGreen).(*types.Func)
	if n := topLevelDecls(c.files, untilGreen); n != 1 || untilObj == nil {
		c.add(c.gen.Pos(), "%s must be defined exactly once at package level, found %d declarations", untilGreen, n)
	}
	for _, f := range c.files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == flakyRunsName {
				if o := c.info.Defs[id]; o != nil && o != types.Object(flakyObj) {
					c.add(id.Pos(), "%s is declared again here: it must stay the one package-level const", flakyRunsName)
				}
			}
			return true
		})
	}
	calls := 0
	walk(c.gen, func(n ast.Node, stack []ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}
		fun, ok := call.Fun.(*ast.Ident)
		if !ok || fun.Name != untilGreen {
			return
		}
		calls++
		c.checkCall(call, fun, parent(stack), flakyObj, untilObj)
	})
	if calls == 0 {
		c.add(c.gen.Pos(), "%s must run a flaky: entry through %s(run, %s)", genFile, untilGreen, flakyRunsName)
	}
	c.checkFailure()
}

func (c *pkgCheck) checkCall(call *ast.CallExpr, fun *ast.Ident, up ast.Node, flakyObj *types.Const, untilObj *types.Func) {
	if untilObj == nil || c.info.Uses[fun] != types.Object(untilObj) {
		c.add(fun.Pos(), "%s here is not the package's function", untilGreen)
	}
	if len(call.Args) != 2 {
		c.add(call.Pos(), "%s takes the replay and flakyRuns", untilGreen)
		return
	}
	arg, ok := call.Args[1].(*ast.Ident)
	if !ok || flakyObj == nil || c.info.Uses[arg] != types.Object(flakyObj) {
		c.add(call.Args[1].Pos(), "the second argument of %s must be the package-level const %s (not a shadowing name or another value)", untilGreen, flakyRunsName)
	}
	if as, ok := up.(*ast.AssignStmt); !ok || len(as.Lhs) != 2 || len(as.Rhs) != 1 || isBlank(as.Lhs[0]) && isBlank(as.Lhs[1]) {
		c.add(call.Pos(), "the result of %s must be kept (diff, err = ...) to be judged", untilGreen)
	}
}

// checkFailure: a flaky: entry is told by its prefix, and the case that errors on a never-green one is there.
func (c *pkgCheck) checkFailure() {
	prefix, failCase := false, false
	ast.Inspect(c.gen, func(n ast.Node) bool {
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
		c.add(c.gen.Pos(), "%s must tell a flaky: entry by strings.HasPrefix(reason, \"flaky:\")", genFile)
	}
	if !failCase {
		c.add(c.gen.Pos(), "%s must fail a flaky: entry that is never green: a case `%s` that errors", genFile, caseWanted)
	}
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
