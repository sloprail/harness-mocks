package main

import (
	"go/ast"
	"go/token"
	"go/types"
)

// failsWhenNeverGreen finds the failing case in fn, after pos.
func (c *pkgCheck) failsWhenNeverGreen(fn ast.Node, pos token.Pos, flaky, errObj, diff types.Object) bool {
	ok := false
	walk(fn, func(n ast.Node, stack []ast.Node) {
		cc, isCase := n.(*ast.CaseClause)
		if !isCase || cc.Pos() < pos || innermostFunc(stack) != fn || c.deadBranch(stack, fn) {
			return
		}
		if len(stack) < 3 {
			return
		}
		sw, isSwitch := stack[len(stack)-3].(*ast.SwitchStmt)
		if !isSwitch || sw.Tag != nil {
			return
		}
		for _, e := range cc.List {
			if c.neverGreenCond(e, flaky, errObj, diff) && c.reachableCase(sw, cc) && c.callsTestingT(cc, stack) {
				ok = true
			}
		}
	})
	return ok
}

// reachableCase: no clause before cc in its switch is a default or a constant true.
func (c *pkgCheck) reachableCase(sw *ast.SwitchStmt, cc *ast.CaseClause) bool {
	for _, st := range sw.Body.List {
		if st == ast.Stmt(cc) {
			return true
		}
		prev := st.(*ast.CaseClause)
		if prev.List == nil {
			return false
		}
		for _, e := range prev.List {
			if tv, ok := c.info.Types[e]; ok && tv.Value != nil && tv.Value.String() == "true" {
				return false
			}
		}
	}
	return false
}

// neverGreenCond: e is `flaky && (err != nil || diff != "")` on exactly these variables.
func (c *pkgCheck) neverGreenCond(e ast.Expr, flaky, errObj, diff types.Object) bool {
	and, ok := e.(*ast.BinaryExpr)
	if !ok || and.Op != token.LAND || !c.is(and.X, flaky) {
		return false
	}
	or, ok := unparen(and.Y).(*ast.BinaryExpr)
	if !ok || or.Op != token.LOR {
		return false
	}
	return c.differs(or.X, errObj, "nil") && c.differs(or.Y, diff, `""`)
}

func (c *pkgCheck) is(e ast.Expr, o types.Object) bool {
	id, ok := unparen(e).(*ast.Ident)
	return ok && c.info.Uses[id] == o
}

// differs: e is `obj != want`, want being nil or the empty string literal.
func (c *pkgCheck) differs(e ast.Expr, obj types.Object, want string) bool {
	b, ok := unparen(e).(*ast.BinaryExpr)
	return ok && b.Op == token.NEQ && c.is(b.X, obj) && types.ExprString(b.Y) == want
}

// callsTestingT: the clause body calls Errorf/Fatalf/Error/Fatal on a *testing.T parameter of a function above it.
func (c *pkgCheck) callsTestingT(cc *ast.CaseClause, stack []ast.Node) bool {
	found := false
	for _, st := range cc.Body {
		ast.Inspect(st, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			recv, isID := sel.X.(*ast.Ident)
			if !isID {
				return true
			}
			switch sel.Sel.Name {
			case "Errorf", "Fatalf", "Error", "Fatal":
				if c.isTestingParam(c.info.Uses[recv], stack) {
					found = true
				}
			}
			return true
		})
	}
	return found
}

// isTestingParam: o is a parameter declared `*testing.T` by a function in stack.
func (c *pkgCheck) isTestingParam(o types.Object, stack []ast.Node) bool {
	for _, n := range stack {
		var ft *ast.FuncType
		switch f := n.(type) {
		case *ast.FuncDecl:
			ft = f.Type
		case *ast.FuncLit:
			ft = f.Type
		default:
			continue
		}
		for _, field := range ft.Params.List {
			if types.ExprString(field.Type) != "*testing.T" {
				continue
			}
			for _, name := range field.Names {
				if o != nil && c.info.Defs[name] == o {
					return true
				}
			}
		}
	}
	return false
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// innermostFunc is the nearest function (declaration or literal) in stack.
func innermostFunc(stack []ast.Node) ast.Node {
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i].(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return stack[i]
		}
	}
	return nil
}
