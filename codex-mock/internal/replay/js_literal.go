package replay

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dop251/goja/ast"
)

// member is left.name: a property of an object the script wrote, of a call's answer, or of something opaque.
func (r *jsRun) member(left ast.Expression, name string) (any, error) {
	v, err := r.eval(left)
	if err != nil {
		return nil, err
	}
	switch o := v.(type) {
	case map[string]any:
		return o[name], nil
	case ref:
		return ref{o.call, o.path + "." + name}, nil
	}
	return opaque{}, nil
}

func (r *jsRun) template(t *ast.TemplateLiteral) (any, error) {
	var b strings.Builder
	for i, el := range t.Elements {
		if !el.Valid {
			return nil, fmt.Errorf("the model's script has a template literal with an invalid escape")
		}
		b.WriteString(el.Parsed.String())
		if i < len(t.Expressions) {
			v, err := r.eval(t.Expressions[i])
			if err != nil {
				return nil, err
			}
			switch s := v.(type) {
			case string:
				b.WriteString(s)
			case number:
				b.WriteString(strconv.FormatFloat(s.f, 'f', -1, 64))
			default:
				return opaque{}, nil
			}
		}
	}
	return b.String(), nil
}

func (r *jsRun) object(o *ast.ObjectLiteral) (any, error) {
	out := map[string]any{}
	for _, p := range o.Value {
		var key string
		var val ast.Expression
		switch pp := p.(type) {
		case *ast.PropertyShort:
			key, val = pp.Name.Name.String(), &pp.Name
		case *ast.PropertyKeyed:
			if pp.Computed {
				return nil, fmt.Errorf("the model's script has a computed property name")
			}
			switch k := pp.Key.(type) {
			case *ast.StringLiteral:
				key = k.Value.String()
			case *ast.Identifier:
				key = k.Name.String()
			default:
				return nil, fmt.Errorf("the model's script has a property name that is a %T", pp.Key)
			}
			val = pp.Value
		default:
			return nil, fmt.Errorf("the model's script has a %T in an object", p)
		}
		v, err := r.eval(val)
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
	return out, nil
}

// function is a function the script wrote: not followed, but it may not call a
// tool (its body is read as code that may not run, with its parameters opaque).
func (r *jsRun) function(params *ast.ParameterList, body any) error {
	names := map[string]any{}
	if params != nil {
		for _, b := range params.List {
			id, ok := b.Target.(*ast.Identifier)
			if !ok {
				return fmt.Errorf("the model's script has a function with a destructured parameter")
			}
			names[id.Name.String()] = opaque{}
		}
	}
	r.cond++
	defer func() { r.cond-- }()
	switch b := body.(type) {
	case *ast.BlockStatement:
		return r.block(b.List, names)
	case *ast.ExpressionBody:
		r.scope = append(r.scope, names)
		defer func() { r.scope = r.scope[:len(r.scope)-1] }()
		_, err := r.eval(b.Expression)
		return err
	}
	return fmt.Errorf("the model's script has a function body that is a %T", body)
}
