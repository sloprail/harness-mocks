package replay

import (
	"fmt"
	"strings"

	"github.com/dop251/goja/ast"
)

// opaque is a value the adapter does not follow: what a script does with a
// tool's answer, the tool list, a function.
type opaque struct{}

// ref is a part of the answer to a tool call of the current rollout:
// call is its number among the rollout's calls, path the properties read from the answer.
type ref struct {
	call int
	path string
}

// number is a JS number the script wrote.
type number struct{ f float64 }

func (r *jsRun) eval(e ast.Expression) (any, error) {
	switch x := e.(type) {
	case *ast.StringLiteral:
		if v := x.Value.String(); !strings.ContainsRune(v, '\uFFFD') || strings.ContainsRune(x.Literal, '\uFFFD') {
			return v, nil
		}
		return nil, fmt.Errorf("the model's script has a string with a lone surrogate")
	case *ast.NumberLiteral:
		switch n := x.Value.(type) {
		case int64:
			return number{float64(n)}, nil
		case float64:
			return number{n}, nil
		}
	case *ast.BooleanLiteral:
		return x.Value, nil
	case *ast.NullLiteral:
		return nil, nil
	case *ast.RegExpLiteral:
		return opaque{}, nil
	case *ast.ArrowFunctionLiteral:
		return opaque{}, r.function(x.ParameterList, x.Body)
	case *ast.FunctionLiteral:
		return opaque{}, r.function(x.ParameterList, x.Body)
	case *ast.TemplateLiteral:
		return r.template(x)
	case *ast.ObjectLiteral:
		return r.object(x)
	case *ast.ArrayLiteral:
		out := make([]any, len(x.Value))
		for i, el := range x.Value {
			v, err := r.eval(el)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case *ast.Identifier:
		return r.ident(x.Name.String())
	case *ast.DotExpression:
		left, err := r.eval(x.Left)
		if err != nil {
			return nil, err
		}
		if err := r.readable(left, x.Identifier.Name.String()); err != nil {
			return nil, err
		}
		return propertyOf(left, x.Identifier.Name.String())
	case *ast.BracketExpression:
		left, err := r.eval(x.Left) // the object before the key, as JS does
		if err != nil {
			return nil, err
		}
		key, err := r.eval(x.Member)
		if err != nil {
			return nil, err
		}
		if err := r.readable(left, "[]"); err != nil {
			return nil, err
		}
		if k, ok := key.(string); ok {
			return propertyOf(left, k)
		}
		return opaque{}, nil
	case *ast.OptionalChain:
		return r.maybe(x.Expression)
	case *ast.Optional:
		return r.maybe(x.Expression)
	case *ast.AwaitExpression:
		return r.eval(x.Argument)
	case *ast.BinaryExpression:
		if _, err := r.eval(x.Left); err != nil {
			return nil, err
		}
		r.cond++ // the right side of ||, ?? and && may not run
		defer func() { r.cond-- }()
		_, err := r.eval(x.Right)
		return opaque{}, err
	case *ast.CallExpression:
		return r.call(x)
	}
	return nil, fmt.Errorf("the model's script has a %T, which the adapter does not read", e)
}

func (r *jsRun) ident(name string) (any, error) {
	if v, ok := r.lookup(name); ok {
		if v == any(unset{}) {
			return nil, fmt.Errorf("the model's script reads %s before it is declared", name)
		}
		return v, nil
	}
	switch name {
	case "undefined":
		return nil, nil
	case "ALL_TOOLS", "JSON", "text", "store", "load":
		return opaque{}, nil
	case "tools":
		return nil, fmt.Errorf("the model's script uses tools other than as tools.<name>(...)")
	}
	return nil, fmt.Errorf("the model's script names %s, which the adapter does not know", name)
}
