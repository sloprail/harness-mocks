package replay

import (
	"fmt"

	"github.com/dop251/goja/ast"
)

// declareAll is `const [a, b] = await Promise.all([f(), g()])`: the calls are made in the
// order of the array (a replay makes them one after the other: the recorded order
// of their results is not the order they were started in, and is not read), and each
// name is bound to the answer of its own call. Any other use of Promise.all is refused.
func (r *jsRun) declareAll(pat *ast.ArrayPattern, init ast.Expression) error {
	await, ok := init.(*ast.AwaitExpression)
	if !ok || pat.Rest != nil {
		return fmt.Errorf("the model's script destructures something other than an awaited Promise.all")
	}
	call, ok := await.Argument.(*ast.CallExpression)
	if !ok || len(call.ArgumentList) != 1 || !isPromiseAll(call.Callee) {
		return fmt.Errorf("the model's script destructures something other than an awaited Promise.all")
	}
	list, ok := call.ArgumentList[0].(*ast.ArrayLiteral)
	if !ok || len(list.Value) != len(pat.Elements) {
		return fmt.Errorf("the model's script destructures a Promise.all of an array that is not as long as its pattern")
	}
	vals := make([]any, len(list.Value))
	for i, e := range list.Value {
		v, err := r.eval(e)
		if err != nil {
			return err
		}
		vals[i] = v
	}
	for i, el := range pat.Elements {
		id, ok := el.(*ast.Identifier)
		if !ok {
			return fmt.Errorf("the model's script destructures a Promise.all into something other than names")
		}
		if err := r.bind(id.Name.String(), vals[i]); err != nil {
			return err
		}
	}
	return nil
}

func isPromiseAll(callee ast.Expression) bool {
	d, ok := callee.(*ast.DotExpression)
	if !ok {
		return false
	}
	id, ok := d.Left.(*ast.Identifier)
	return ok && id.Name == "Promise" && d.Identifier.Name == "all"
}

// bind declares a name in the innermost scope.
func (r *jsRun) bind(name string, v any) error {
	top := r.scope[len(r.scope)-1]
	if old, again := top[name]; (again && old != any(unset{})) || reserved[name] {
		return fmt.Errorf("the model's script declares %s, which is the harness's or already declared", name)
	}
	if r.cond > 0 { // declared by code that may not run: its value is not known
		v = opaque{}
	}
	top[name] = v
	return nil
}
