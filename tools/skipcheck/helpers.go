package main

import (
	"go/ast"
	"go/types"
)

func isLookPath(fn *types.Func) bool {
	return fn.Pkg() != nil && fn.Pkg().Path() == "os/exec" && fn.Name() == "LookPath"
}

// isSkip: a Skip, Skipf or SkipNow method of the testing package (T, B, F, TB and a generic instance of them),
// or one whose receiver is an interface or a type parameter wherever it is declared (a local interface, a generic
// constraint): such a method can be (*testing.T).Skip behind the interface, so it counts as one.
func isSkip(fn *types.Func) bool {
	fn = fn.Origin()
	switch fn.Name() {
	case "Skip", "Skipf", "SkipNow":
	default:
		return false
	}
	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return false
	}
	if fn.Pkg() != nil && fn.Pkg().Path() == "testing" {
		return true
	}
	t := recv.Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	if _, ok := t.(*types.TypeParam); ok {
		return true
	}
	_, isIface := t.Underlying().(*types.Interface)
	return isIface
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
