package main

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"regexp"
)

var gateName = regexp.MustCompile(`^A10N_[A-Z0-9]+(_[A-Z0-9]+)*_TEST$`)

// exempt: id (the last of stack) is the function of a call that is a statement of the body of an `if`
// whose condition is exactly an os.Getenv("A10N_<NAME>_TEST") comparison with a constant: an explicit
// opt-in gate. A method value, a call in a larger expression or a condition with anything else is not.
func exempt(id *ast.Ident, stack []ast.Node, info *types.Info) bool {
	i := len(stack) - 2
	if i >= 0 {
		if sel, ok := stack[i].(*ast.SelectorExpr); ok && sel.Sel == id {
			i--
		}
	}
	if i < 3 {
		return false
	}
	call, ok := stack[i].(*ast.CallExpr)
	if !ok || unparen(call.Fun) != lastOf(stack[i+1:]) {
		return false
	}
	if _, ok := stack[i-1].(*ast.ExprStmt); !ok {
		return false
	}
	block, ok := stack[i-2].(*ast.BlockStmt)
	if !ok {
		return false
	}
	is, ok := stack[i-3].(*ast.IfStmt)
	return ok && is.Body == block && is.Init == nil && gate(is.Cond, info)
}

func lastOf(nodes []ast.Node) ast.Node {
	if len(nodes) == 0 {
		return nil
	}
	if sel, ok := nodes[0].(*ast.SelectorExpr); ok {
		return sel
	}
	return nodes[0]
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

// gate: cond is `os.Getenv("A10N_X_TEST") == const` or `!= const`, and nothing else.
func gate(cond ast.Expr, info *types.Info) bool {
	b, ok := unparen(cond).(*ast.BinaryExpr)
	if !ok || (b.Op != token.EQL && b.Op != token.NEQ) {
		return false
	}
	return (getenv(b.X, info) && isConst(b.Y, info)) || (getenv(b.Y, info) && isConst(b.X, info))
}

func isConst(e ast.Expr, info *types.Info) bool {
	tv, ok := info.Types[e]
	return ok && tv.Value != nil
}

// getenv: e is a call of os.Getenv with an A10N_<NAME>_TEST string constant.
func getenv(e ast.Expr, info *types.Info) bool {
	call, ok := unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	var id *ast.Ident
	switch f := unparen(call.Fun).(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return false
	}
	fn, ok := info.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "os" || fn.Name() != "Getenv" {
		return false
	}
	tv, ok := info.Types[call.Args[0]]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.String && gateName.MatchString(constant.StringVal(tv.Value))
}

// isGateEnv: os.Setenv, os.Unsetenv, or the Setenv method of a testing type.
func isGateEnv(fn *types.Func) bool {
	fn = fn.Origin()
	if fn.Pkg() == nil {
		return false
	}
	switch fn.Pkg().Path() + "." + fn.Name() {
	case "os.Setenv", "os.Unsetenv", "testing.Setenv":
		return true
	}
	return false
}

// setsGate: the call whose function is id has an A10N_<NAME>_TEST string constant as its first argument.
func setsGate(id *ast.Ident, stack []ast.Node, info *types.Info) bool {
	i := len(stack) - 2
	if i >= 0 {
		if sel, ok := stack[i].(*ast.SelectorExpr); ok && sel.Sel == id {
			i--
		}
	}
	if i < 0 {
		return false
	}
	call, ok := stack[i].(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	tv, ok := info.Types[call.Args[0]]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.String && gateName.MatchString(constant.StringVal(tv.Value))
}
