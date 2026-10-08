package replay

import (
	"fmt"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
)

// parseScript parses the JavaScript of one of the model's `exec` calls. Codex
// runs it as the body of an async function (it awaits at the top level), so it
// is parsed as one; the statements of that body are returned.
func parseScript(js string) ([]ast.Statement, error) {
	prog, err := parser.ParseFile(nil, "", "async function __exec() {\n"+js+"\n}", 0)
	if err != nil {
		return nil, fmt.Errorf("the model's script is not JavaScript the adapter can parse: %w", err)
	}
	if len(prog.Body) != 1 {
		return nil, fmt.Errorf("the model's script closes its own body")
	}
	decl, ok := prog.Body[0].(*ast.FunctionDeclaration)
	if !ok {
		return nil, fmt.Errorf("the model's script closes its own body")
	}
	return decl.Function.Body.List, nil
}
