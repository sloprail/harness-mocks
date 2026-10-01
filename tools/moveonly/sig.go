package main

import (
	"go/ast"
	"path"
	"strings"
)

var knownSuffixes = strings.Fields(`aix android darwin dragonfly freebsd hurd illumos ios js linux nacl
netbsd openbsd plan9 solaris wasip1 windows zos 386 amd64 arm arm64 loong64 mips
mipsle mips64 mips64le ppc64 ppc64le riscv64 s390x sparc64 wasm test`)

// fileSig captures everything about a file that makes a declaration behave
// differently once it lands in another file: the package name, the build
// constraints written above the package clause, and a _test/GOOS/GOARCH
// file-name suffix. It is part of every item's identity.
func fileSig(name string, f *ast.File) string {
	parts := []string{f.Name.Name}
	for _, g := range f.Comments {
		if g.End() > f.Package {
			break
		}
		for _, c := range g.List {
			if strings.HasPrefix(c.Text, "//go:build") || strings.HasPrefix(c.Text, "// +build") {
				parts = append(parts, c.Text)
			}
		}
	}
	base := strings.TrimSuffix(path.Base(name), ".go")
	segs := strings.Split(base, "_")
	for i := len(segs) - 1; i >= 1 && i >= len(segs)-2; i-- {
		for _, k := range knownSuffixes {
			if segs[i] == k {
				parts = append(parts, "suffix:"+k)
			}
		}
	}
	return strings.Join(parts, ";")
}
