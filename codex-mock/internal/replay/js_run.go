package replay

import (
	"fmt"

	"github.com/dop251/goja/ast"
)

// jsCall is a call of one of the harness's tools (`tools.<name>(args)`) that a
// model script made, with its arguments evaluated.
type jsCall struct {
	Name string
	Args []any
}

// jsRun evaluates the model's scripts of one rollout, one after the other: only
// what decides the tool calls is evaluated (literals, constants, objects,
// arrays, templates, the receipts of earlier calls, store/load). What a
// script does with a result (text(...), ALL_TOOLS.filter(...)) is opaque. A script whose
// calls depend on what a tool answered (a call under an `if`) is an error: it
// cannot be replayed from the recording of one run.
type jsRun struct {
	store map[string]any   // store(k, v) / load(k): kept across the rollout's scripts
	scope []map[string]any // the current script's names, innermost last
	calls []jsCall         // the current script's tool calls, in the order it made them
	cond  int              // > 0 inside code that may not run
	n     int              // the tool calls of the rollout so far: a ref names one by its number
}

func newJSRun() *jsRun { return &jsRun{store: map[string]any{}} }

// script evaluates one script and returns the tool calls it made, in order.
func (r *jsRun) script(js string) ([]jsCall, error) {
	body, err := parseScript(js)
	if err != nil {
		return nil, err
	}
	r.scope, r.calls = []map[string]any{{}}, nil
	for _, st := range body {
		if err := r.stmt(st); err != nil {
			return nil, err
		}
	}
	return r.calls, nil
}

func (r *jsRun) stmt(st ast.Statement) error {
	switch s := st.(type) {
	case *ast.ExpressionStatement:
		_, err := r.eval(s.Expression)
		return err
	case *ast.LexicalDeclaration:
		return r.declare(s.List)
	case *ast.IfStatement:
		if _, err := r.eval(s.Test); err != nil {
			return err
		}
		r.cond++
		defer func() { r.cond-- }()
		if err := r.stmt(s.Consequent); err != nil || s.Alternate == nil {
			return err
		}
		return r.stmt(s.Alternate)
	case *ast.BlockStatement:
		return r.block(s.List, nil)
	}
	return fmt.Errorf("the model's script has a %T, which the adapter does not read", st)
}

// block runs statements in a scope of their own, with the given names.
func (r *jsRun) block(list []ast.Statement, names map[string]any) error {
	if names == nil {
		names = map[string]any{}
	}
	r.scope = append(r.scope, names)
	defer func() { r.scope = r.scope[:len(r.scope)-1] }()
	for _, st := range list {
		if err := r.stmt(st); err != nil {
			return err
		}
	}
	return nil
}

// reserved are the names a script gets from its harness; it may not declare them.
var reserved = map[string]bool{"tools": true, "store": true, "load": true, "text": true, "ALL_TOOLS": true, "JSON": true, "undefined": true}

func (r *jsRun) lookup(name string) (any, bool) {
	for i := len(r.scope) - 1; i >= 0; i-- {
		if v, ok := r.scope[i][name]; ok {
			return v, true
		}
	}
	return nil, false
}

func (r *jsRun) declare(list []*ast.Binding) error {
	for _, b := range list {
		id, ok := b.Target.(*ast.Identifier)
		if !ok || b.Initializer == nil {
			return fmt.Errorf("the model's script declares something other than name = value")
		}
		v, err := r.eval(b.Initializer)
		if err != nil {
			return err
		}
		name := id.Name.String()
		top := r.scope[len(r.scope)-1]
		if _, again := top[name]; again || reserved[name] {
			return fmt.Errorf("the model's script declares %s, which is the harness's or already declared", name)
		}
		if r.cond > 0 { // declared by code that may not run: its value is not known
			v = opaque{}
		}
		top[name] = v
	}
	return nil
}
