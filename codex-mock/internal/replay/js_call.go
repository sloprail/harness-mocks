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
	var recv any // JS evaluates a method's receiver before its arguments
	d, isDot := c.Callee.(*ast.DotExpression)
	if _, isTool := toolName(c.Callee); isDot && !isTool {
		var err error
		if recv, err = r.eval(d.Left); err != nil {
			return nil, err
		}
	}
	for i, a := range c.ArgumentList {
		v, err := r.eval(a)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	if tool, ok := toolName(c.Callee); ok {
		r.calls = append(r.calls, jsCall{Num: r.n, Name: tool, Args: args})
		r.n++
		return ref{call: r.n - 1}, nil
	}
	if id, ok := c.Callee.(*ast.Identifier); ok {
		switch id.Name.String() {
		case "store":
			if r.cond > 0 {
				return nil, fmt.Errorf("a store that depends on what an earlier tool answered")
			}
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
	if isDot {
		if err := r.readable(recv, d.Identifier.Name.String()); err != nil {
			return nil, err
		}
	}
	if isDot { // a method of something opaque (ALL_TOOLS.filter, JSON.stringify)
		switch recv.(type) {
		case []any, map[string]any: // a method may change what the script wrote (reverse, push), which is not followed
			return nil, fmt.Errorf("the model's script calls %s on a value it wrote, which the adapter does not follow", d.Identifier.Name)
		}
		return opaque{}, nil
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
