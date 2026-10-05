package replay

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/dop251/goja/ast"
)

// readable refuses reading a property of null or undefined: JS throws there and the
// script ends, so what follows never ran. Code that may not run (cond) is not refused.
// Only a read of a null the adapter holds is known: a read of a value it does not
// follow (a script-written array's element, say) is not refused even where JS would throw.
func (r *jsRun) readable(v any, name string) error {
	if v == nil && r.cond == 0 {
		return fmt.Errorf("the model's script reads %s of null or undefined, which throws", name)
	}
	return nil
}

// propertyOf is v.name: a property of an object the script wrote, of a call's answer, or of something opaque.
func propertyOf(v any, name string) (any, error) {
	switch o := v.(type) {
	case map[string]any:
		if _, own := o[name]; !own && objectProto[name] {
			return nil, fmt.Errorf("the model's script reads %s of an object, which is JavaScript's own", name)
		}
		return o[name], nil
	case ref:
		return ref{o.call, o.path + "." + name}, nil
	}
	return opaque{}, nil
}

// objectProto are the properties every JS object has without the script writing them.
var objectProto = map[string]bool{"toString": true, "constructor": true, "hasOwnProperty": true, "valueOf": true, "__proto__": true,
	"isPrototypeOf": true, "__defineGetter__": true, "__defineSetter__": true, "__lookupGetter__": true, "__lookupSetter__": true, "propertyIsEnumerable": true, "toLocaleString": true}

func (r *jsRun) template(t *ast.TemplateLiteral) (any, error) {
	if t.Tag != nil {
		return nil, fmt.Errorf("the model's script has a tagged template, whose tag may do anything")
	}
	var b strings.Builder
	for i, el := range t.Elements {
		if !el.Valid {
			return nil, fmt.Errorf("the model's script has a template literal with an invalid escape")
		}
		if s := el.Parsed.String(); strings.ContainsRune(s, '\uFFFD') && !strings.ContainsRune(el.Literal, '\uFFFD') {
			return nil, fmt.Errorf("the model's script has a string with a lone surrogate")
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
				if a := math.Abs(s.f); a >= 1e21 || a != 0 && a < 1e-6 {
					return nil, fmt.Errorf("the model's script writes a number JavaScript prints in exponent form")
				}
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
			if key == "__proto__" {
				return nil, fmt.Errorf("the model's script sets __proto__")
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
		if params.Rest != nil {
			return fmt.Errorf("the model's script has a function with a rest parameter")
		}
		for _, b := range params.List {
			id, ok := b.Target.(*ast.Identifier)
			if !ok || b.Initializer != nil {
				return fmt.Errorf("the model's script has a function with a destructured or defaulted parameter")
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
