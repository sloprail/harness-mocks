package replay

import (
	"fmt"

	"github.com/dop251/goja/ast"
)

// call evaluates a call: of one of the harness's tools (recorded), of
// store/load (the script's memory across calls of the rollout), or of anything
// else, which is the script looking at what it was given (opaque).
func (r *jsRun) call(c *ast.CallExpression) (any, error) {
	args := make([]any, len(c.ArgumentList))
	if tool, ok := toolName(c.Callee); ok && r.cond > 0 {
		return nil, fmt.Errorf("a call of tools.%s that depends on what an earlier tool answered", tool)
	}
	for i, a := range c.ArgumentList {
		v, err := r.eval(a)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	if tool, ok := toolName(c.Callee); ok {
		r.calls = append(r.calls, jsCall{Name: tool, Args: args})
		r.n++
		return ref{call: r.n - 1}, nil
	}
	if id, ok := c.Callee.(*ast.Identifier); ok {
		switch id.Name.String() {
		case "store":
			if k, ok := argString(args, 0); ok && len(args) == 2 {
				r.store[k] = args[1]
				return nil, nil
			}
			return nil, fmt.Errorf("a store call whose key is not a string")
		case "load":
			k, _ := argString(args, 0)
			v, ok := r.store[k]
			if !ok {
				return nil, fmt.Errorf("a load of %q, which no script stored", k)
			}
			return v, nil
		case "text":
			return opaque{}, nil
		}
		return nil, fmt.Errorf("the model's script calls %s, which the adapter does not know", id.Name)
	}
	if d, ok := c.Callee.(*ast.DotExpression); ok { // a method of something opaque (ALL_TOOLS.filter, JSON.stringify)
		_, err := r.eval(d.Left)
		return opaque{}, err
	}
	return nil, fmt.Errorf("the model's script calls a %T, which the adapter does not read", c.Callee)
}

// toolName is the name of the tool a callee `tools.<name>` is.
func toolName(callee ast.Expression) (string, bool) {
	d, ok := callee.(*ast.DotExpression)
	if !ok {
		return "", false
	}
	id, ok := d.Left.(*ast.Identifier)
	if !ok || id.Name != "tools" {
		return "", false
	}
	return d.Identifier.Name.String(), true
}

func argString(args []any, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	s, ok := args[i].(string)
	return s, ok
}
