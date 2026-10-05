package main

import (
	"go/ast"
	"go/token"
	"go/types"
)

// checkUntilShape: replayUntilGreen(run, attempts) really repeats. Its body has a loop bounded by the
// attempts parameter that calls the run parameter and stops when a run is green (the loop condition or
// an if in the loop tests an error against nil or a diff against ""), and it returns the last result.
// A stub such as `return run()` is refused.
func (c *pkgCheck) checkUntilShape(_ *types.Func) {
	var fd *ast.FuncDecl
	for _, f := range c.files {
		for _, d := range f.Decls {
			if x, ok := d.(*ast.FuncDecl); ok && x.Recv == nil && x.Name.Name == untilGreen {
				fd = x
			}
		}
	}
	if fd == nil || fd.Body == nil {
		return
	}
	var params []types.Object
	for _, field := range fd.Type.Params.List {
		for _, n := range field.Names {
			params = append(params, c.info.Defs[n])
		}
	}
	if len(params) != 2 || params[0] == nil || params[1] == nil {
		c.add(fd.Pos(), "%s must take (run, attempts)", untilGreen)
		return
	}
	run, attempts := params[0], params[1]
	repeats, stops := false, false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		loop, ok := n.(*ast.ForStmt)
		if !ok || loop.Cond == nil || !c.mentions(loop.Cond, attempts) || !c.calls(loop.Body, run) {
			return true
		}
		repeats = true
		if c.testsGreen(loop.Cond) || c.returnsWhenGreen(loop.Body) {
			stops = true
		}
		return true
	})
	if !repeats {
		c.add(fd.Pos(), "%s must loop up to its attempts parameter, calling the run parameter in the loop", untilGreen)
	} else if !stops {
		c.add(fd.Pos(), "%s must stop looping once a run is green (an error compared with nil, or a diff with \"\")", untilGreen)
	}
}

func (c *pkgCheck) mentions(n ast.Node, o types.Object) bool {
	found := false
	ast.Inspect(n, func(m ast.Node) bool {
		if id, ok := m.(*ast.Ident); ok && c.info.Uses[id] == o {
			found = true
		}
		return true
	})
	return found
}

// calls: n holds a call of the function variable o.
func (c *pkgCheck) calls(n ast.Node, o types.Object) bool {
	found := false
	ast.Inspect(n, func(m ast.Node) bool {
		if call, ok := m.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && c.info.Uses[id] == o {
				found = true
			}
		}
		return true
	})
	return found
}

// testsGreen: e compares something with nil or with the empty string.
func (c *pkgCheck) testsGreen(e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(m ast.Node) bool {
		if b, ok := m.(*ast.BinaryExpr); ok && (b.Op == token.EQL || b.Op == token.NEQ) {
			if s := types.ExprString(b.Y); s == "nil" || s == `""` {
				found = true
			}
		}
		return true
	})
	return found
}

// returnsWhenGreen: the loop body has an if that tests a green result and returns.
func (c *pkgCheck) returnsWhenGreen(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(m ast.Node) bool {
		if is, ok := m.(*ast.IfStmt); ok && c.testsGreen(is.Cond) {
			ast.Inspect(is.Body, func(r ast.Node) bool {
				if _, ok := r.(*ast.ReturnStmt); ok {
					found = true
				}
				return true
			})
		}
		return true
	})
	return found
}
