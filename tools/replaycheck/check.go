package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// fakeImporter answers every import with an empty package: the files are checked on their own, so
// what an import provides is not known, but every name the package itself declares resolves.
type fakeImporter struct{}

func (fakeImporter) Import(path string) (*types.Package, error) {
	name := path[strings.LastIndex(path, "/")+1:]
	p := types.NewPackage(path, strings.ReplaceAll(name, "-", "_"))
	p.MarkComplete()
	return p, nil
}

var _ types.Importer = fakeImporter{}

// checkDir judges the Go package files of dir:
//
//	(a) the list file declares notReplaying (see entriesOf)
//	(b) the generated test only reads it: an index expression that is not assigned, or a range
//	(c) flakyRuns is the package-level const 3 where replayUntilGreen is called, which is declared once
//	(d) no other file of the package names notReplaying
func checkDir(dir string) ([]string, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	fset := token.NewFileSet()
	groups := map[string][]*ast.File{}
	var violations []string
	for _, n := range names {
		src, err := os.ReadFile(n)
		if err != nil {
			return nil, err
		}
		f, perr := parser.ParseFile(fset, n, src, 0)
		if perr != nil {
			violations = append(violations, fmt.Sprintf("%s does not parse: %v", n, perr))
			continue
		}
		groups[f.Name.Name] = append(groups[f.Name.Name], f)
	}
	var pkgs []string
	for p := range groups {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		c := &pkgCheck{fset: fset, files: groups[p]}
		if err := c.run(p); err != nil {
			return nil, err
		}
		violations = append(violations, c.out...)
	}
	return violations, nil
}

// pkgCheck is the judgement of one package of the directory.
type pkgCheck struct {
	fset      *token.FileSet
	files     []*ast.File
	list, gen *ast.File // the list file and the generated test, nil when the package has none
	info      *types.Info
	pkg       *types.Package
	out       []string
}

func (c *pkgCheck) add(pos token.Pos, format string, args ...any) {
	c.out = append(c.out, fmt.Sprintf("%s: %s", c.fset.Position(pos), fmt.Sprintf(format, args...)))
}

func (c *pkgCheck) run(pkgName string) error {
	for _, f := range c.files {
		switch filepath.Base(c.fset.Position(f.Pos()).Filename) {
		case listFile:
			c.list = f
		case genFile:
			c.gen = f
		}
	}
	c.checkNaming()
	if c.list == nil && c.gen == nil {
		return nil
	}
	if c.list == nil {
		if mentions(c.gen, listName) {
			c.add(c.gen.Pos(), "%s reads %s, but %s is gone or renamed: keep the list in %s", genFile, listName, listFile, listFile)
		}
		return nil
	}
	if c.gen == nil {
		return nil
	}
	c.info = &types.Info{Uses: map[*ast.Ident]types.Object{}, Defs: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: fakeImporter{}, Error: func(error) {}, FakeImportC: true}
	c.pkg, _ = conf.Check(pkgName, c.fset, c.files, c.info)
	if c.pkg == nil {
		return fmt.Errorf("type-checking %s produced no package", pkgName)
	}
	if c.checkReads() {
		c.checkFlaky()
	}
	return nil
}

// checkNaming is (d): the list is named only in its own two files (a shadowing or aliasing
// declaration included).
func (c *pkgCheck) checkNaming() {
	for _, f := range c.files {
		if f == c.list || f == c.gen {
			continue
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == listName {
				c.add(id.Pos(), "%s is named outside %s and %s, so an entry could be added there unseen", listName, listFile, genFile)
			}
			return true
		})
	}
}

func mentions(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return true
	})
	return found
}
