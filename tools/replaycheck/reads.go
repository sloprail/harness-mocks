package main

import (
	"go/ast"
	"go/token"
)

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

// skipParens climbs from stack[i] through the parentheses around it: the index of the outermost
// ParenExpr (or i itself when it has none).
func skipParens(stack []ast.Node, i int) int {
	for i > 0 {
		if _, ok := stack[i-1].(*ast.ParenExpr); !ok {
			break
		}
		i--
	}
	return i
}

// isRead is a whitelist. id (the last of stack) may be the operand of a range, or the operand of an
// index expression that stands where a value is only read; every other use (an assignment target, an
// increment, an address, a range target, a delete or any call with the map itself, an alias) is not a read.
// Parentheses around the identifier or around the index expression do not change what it is.
func isRead(id *ast.Ident, stack []ast.Node) bool {
	e := skipParens(stack, len(stack)-1)
	if e == 0 {
		return false
	}
	switch p := stack[e-1].(type) {
	case *ast.RangeStmt:
		return p.X == stack[e].(ast.Expr)
	case *ast.IndexExpr:
		if p.X != stack[e].(ast.Expr) {
			return false
		}
		ie := skipParens(stack, e-1)
		if ie == 0 {
			return false
		}
		return rvalue(stack[ie-1], stack[ie].(ast.Expr))
	}
	return false
}

// rvalue: in parent, expr is where a value is read, not assigned to or addressed.
func rvalue(parent ast.Node, expr ast.Expr) bool {
	switch p := parent.(type) {
	case *ast.AssignStmt:
		for _, r := range p.Rhs {
			if r == expr {
				return true
			}
		}
	case *ast.ValueSpec:
		for _, v := range p.Values {
			if v == expr {
				return true
			}
		}
	case *ast.CallExpr:
		for _, a := range p.Args {
			if a == expr {
				return true
			}
		}
	case *ast.ReturnStmt:
		for _, r := range p.Results {
			if r == expr {
				return true
			}
		}
	case *ast.CompositeLit:
		for _, el := range p.Elts {
			if el == expr {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return p.Value == expr
	case *ast.IfStmt:
		return p.Cond == expr
	case *ast.ForStmt:
		return p.Cond == expr
	case *ast.SwitchStmt:
		return p.Tag == expr
	case *ast.CaseClause:
		for _, v := range p.List {
			if v == expr {
				return true
			}
		}
	case *ast.BinaryExpr:
		return true
	case *ast.UnaryExpr:
		return p.Op != token.AND
	case *ast.IndexExpr:
		return p.Index == expr
	}
	return false
}
