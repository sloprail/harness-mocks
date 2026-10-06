package main

import (
	"go/ast"
	"go/types"
)

// collect makes a unit of every function declaration with a body, keyed by its object.
func collect(files []*ast.File, info *types.Info) map[*types.Func]*unit {
	units := map[*types.Func]*unit{}
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			obj, _ := info.Defs[fd.Name].(*types.Func)
			if obj == nil {
				continue
			}
			u := &unit{fd: fd, obj: obj, refs: map[*types.Func]bool{}}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					if fn, ok := info.Uses[id].(*types.Func); ok {
						u.refs[fn] = true
					}
				}
				return true
			})
			units[obj] = u
		}
	}
	return units
}

// propagate marks the units that reach LookPath and the units that skip, through the functions they name.
func propagate(units map[*types.Func]*unit) {
	for changed := true; changed; {
		changed = false
		for _, u := range units {
			for fn := range u.refs {
				lookup := isLookPath(fn) || (units[fn] != nil && units[fn].lookup)
				skip := isSkip(fn) || (units[fn] != nil && units[fn].skipper)
				if lookup && !u.lookup {
					u.lookup, changed = true, true
				}
				if skip && !u.skipper {
					u.skipper, changed = true, true
				}
			}
		}
	}
}

func isLookPath(fn *types.Func) bool {
	return fn.Pkg() != nil && fn.Pkg().Path() == "os/exec" && fn.Name() == "LookPath"
}

// isSkip: a Skip, Skipf or SkipNow of the testing package (T, B, F, TB: all the same methods).
func isSkip(fn *types.Func) bool {
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
