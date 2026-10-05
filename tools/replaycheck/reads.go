package main

import "go/ast"

// checkReads is (b): the generated test only reads the list. It reports false when the list cannot
// be found at package level, so the rest of the judgement has nothing to stand on.
func (c *pkgCheck) checkReads() bool {
	listObj := c.pkg.Scope().Lookup(listName)
	if listObj == nil {
		c.add(c.list.Pos(), "%s is not declared at package level in %s", listName, listFile)
		return false
	}
	walk(c.gen, func(n ast.Node, stack []ast.Node) {
		id, ok := n.(*ast.Ident)
		if !ok || id.Name != listName {
			return
		}
		if c.info.Uses[id] != listObj {
			c.add(id.Pos(), "%s here is not the package's list (a shadowing name): the generated test may only read the list itself", listName)
			return
		}
		if !isRead(id, stack) {
			c.add(id.Pos(), "the generated test may only read %s (a range, or an index that is not assigned), not change, alias or pass it", listName)
		}
	})
	return true
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
