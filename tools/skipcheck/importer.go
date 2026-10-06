package main

import (
	"go/importer"
	"go/token"
	"go/types"
	"strings"
)

// stdImporter type-checks the standard library from source (so os/exec and testing resolve) and answers
// any other import with an empty package: the files are checked on their own, so a name another module
// provides is unknown, but every name of the standard library and of the package itself resolves.
type stdImporter struct{ src types.Importer }

func newImporter(fset *token.FileSet) types.Importer {
	return stdImporter{src: importer.ForCompiler(fset, "source", nil)}
}

func (s stdImporter) Import(path string) (*types.Package, error) {
	if !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
		if p, err := s.src.Import(path); err == nil {
			return p, nil
		}
	}
	name := path[strings.LastIndex(path, "/")+1:]
	p := types.NewPackage(path, strings.ReplaceAll(name, "-", "_"))
	p.MarkComplete()
	return p, nil
}
