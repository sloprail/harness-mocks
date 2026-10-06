package main

import (
	"go/ast"
	"go/constant"
	"go/types"
)

const (
	flakyRunsName = "flakyRuns"
	untilGreen    = "replayUntilGreen"
)

// checkFlaky is (c): flakyRuns is the one package-level const 3. What runs it, replayUntilGreen and
// the test that fails a never-green flaky: entry, is pinned to its canonical copy (checkPinned).
func (c *pkgCheck) checkFlaky() error {
	flakyObj, _ := c.pkg.Scope().Lookup(flakyRunsName).(*types.Const)
	if flakyObj == nil {
		c.add(c.gen.Pos(), "%s is not a package-level const", flakyRunsName)
	} else if v, ok := constant.Int64Val(constant.ToInt(flakyObj.Val())); !ok || v != 3 {
		c.add(flakyObj.Pos(), "%s must be 3, a flaky: entry is replayed three times", flakyRunsName)
	}
	if n := topLevelDecls(c.files, flakyRunsName); n != 1 {
		c.add(c.gen.Pos(), "%s must be declared exactly once at package level, found %d declarations", flakyRunsName, n)
	}
	for _, f := range c.files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == flakyRunsName {
				if o := c.info.Defs[id]; o != nil && o != types.Object(flakyObj) {
					c.add(id.Pos(), "%s is declared again here: it must stay the one package-level const", flakyRunsName)
				}
			}
			return true
		})
	}
	return c.checkPinned()
}

// topLevelDecls counts the package-level declarations of name (const, var, type, func) over files.
func topLevelDecls(files []*ast.File, name string) int {
	n := 0
	for _, f := range files {
		for _, d := range f.Decls {
			switch x := d.(type) {
			case *ast.FuncDecl:
				if x.Recv == nil && x.Name.Name == name {
					n++
				}
			case *ast.GenDecl:
				for _, s := range x.Specs {
					switch sp := s.(type) {
					case *ast.ValueSpec:
						for _, id := range sp.Names {
							if id.Name == name {
								n++
							}
						}
					case *ast.TypeSpec:
						if sp.Name.Name == name {
							n++
						}
					}
				}
			}
		}
	}
	return n
}
