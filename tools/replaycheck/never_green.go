package main

import (
	"go/ast"
	"go/token"
	"go/types"
)

// checkNeverGreen: a flaky: entry that is never green fails the test. From the statement
// `diff, err = replayUntilGreen(run, flakyRuns)`:
//   - the call is under `if flaky`, and flaky is a define from strings.HasPrefix(reason, "flaky:")
//   - in the same function, after the call, a case of a tagless switch is
//     `flaky && (err != nil || diff != "")` on exactly those variables, reachable, and its body
//     calls Errorf/Fatalf/Error/Fatal on the test's *testing.T (failsWhenNeverGreen)
func (c *pkgCheck) checkNeverGreen(s callSite) {
	diff := c.assigned(s.as.Lhs[0], s.as)
	errObj := c.assigned(s.as.Lhs[1], s.as)
	if diff == nil || errObj == nil {
		c.add(s.call.Pos(), "the variables keeping the result of %s could not be resolved", untilGreen)
		return
	}
	fn := innermostFunc(s.stack)
	if fn == nil {
		c.add(s.call.Pos(), "%s is called outside any function", untilGreen)
		return
	}
	flaky := c.flakyDefine(fn)
	if flaky == nil {
		c.add(fn.Pos(), "no `flaky := ... strings.HasPrefix(reason, \"flaky:\")` define in the function that runs %s", untilGreen)
		return
	}
	if !c.underIf(s.stack, flaky) {
		c.add(s.call.Pos(), "%s must run under `if flaky`, with flaky the define that tests the \"flaky:\" prefix", untilGreen)
	}
	if c.deadBranch(s.stack, fn) {
		c.add(s.call.Pos(), "%s sits under a branch that never runs", untilGreen)
	}
	if !c.failsWhenNeverGreen(fn, s.call.End(), flaky, errObj, diff) {
		c.add(fn.Pos(), "the function that runs %s must, after it, fail a flaky: entry that is never green: a reachable `case flaky && (err != nil || diff != \"\")` of a tagless switch, on those variables, that calls Errorf or Fatalf on the test's *testing.T", untilGreen)
	}
}

// assigned is the variable an assignment target names: used by `=`, defined by `:=`.
func (c *pkgCheck) assigned(e ast.Expr, as *ast.AssignStmt) types.Object {
	id, ok := e.(*ast.Ident)
	if !ok {
		return nil
	}
	if as.Tok == token.DEFINE {
		return c.info.Defs[id]
	}
	return c.info.Uses[id]
}

// flakyDefine is the object defined by `flaky := ... strings.HasPrefix(x, "flaky:")` in fn.
func (c *pkgCheck) flakyDefine(fn ast.Node) types.Object {
	var found types.Object
	walk(fn, func(n ast.Node, stack []ast.Node) {
		as, ok := n.(*ast.AssignStmt)
		if !ok || as.Tok != token.DEFINE || len(as.Lhs) != 1 || len(as.Rhs) != 1 || innermostFunc(stack) != fn {
			return
		}
		id, ok := as.Lhs[0].(*ast.Ident)
		if !ok || id.Name != "flaky" {
			return
		}
		ast.Inspect(as.Rhs[0], func(m ast.Node) bool {
			if call, ok := m.(*ast.CallExpr); ok && len(call.Args) == 2 {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "HasPrefix" {
					if lit, ok := stringLit(call.Args[1]); ok && lit == "flaky:" {
						found = c.info.Defs[id]
					}
				}
			}
			return true
		})
	})
	return found
}

// underIf: some `if` above the last node of stack has the object as its whole condition and holds the node in its body.
func (c *pkgCheck) underIf(stack []ast.Node, cond types.Object) bool {
	for i := len(stack) - 2; i >= 0; i-- {
		if is, ok := stack[i].(*ast.IfStmt); ok && is.Body == stack[i+1] {
			if id, ok := is.Cond.(*ast.Ident); ok && c.info.Uses[id] == cond {
				return true
			}
		}
	}
	return false
}

// deadBranch: between the node and fn, an if or for condition is a constant false.
func (c *pkgCheck) deadBranch(stack []ast.Node, fn ast.Node) bool {
	for i := len(stack) - 1; i >= 0 && stack[i] != fn; i-- {
		var cond ast.Expr
		switch x := stack[i].(type) {
		case *ast.IfStmt:
			cond = x.Cond
		case *ast.ForStmt:
			cond = x.Cond
		}
		if cond != nil {
			if tv, ok := c.info.Types[cond]; ok && tv.Value != nil && tv.Value.String() == "false" {
				return true
			}
		}
	}
	return false
}
