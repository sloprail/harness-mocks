// Command skipcheck refuses a Go test that skips because a tool is missing. It type-checks the
// packages of the directories it is given and reports every use of (*testing.T).Skip, Skipf or SkipNow
// (a call, a method value, or a call of a package helper that skips) in a function that reaches
// os/exec.LookPath, resolved by types so an alias or a dot import of os/exec counts. The one exemption is
// a skip directly under `if os.Getenv("A10N_<NAME>_TEST") <op> <constant>`: an explicit opt-in gate.
//
//	skipcheck DIR...
//
// Exit 0: fine. Exit 1: violations, one per line on stdout. Exit 2: the tool itself failed (stderr).
package main

import (
	"fmt"
	"go/token"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: skipcheck DIR...")
		os.Exit(2)
	}
	fset := token.NewFileSet()
	imp := newImporter(fset)
	bad := 0
	for _, dir := range os.Args[1:] {
		v, err := checkDir(fset, imp, dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "skipcheck:", err)
			os.Exit(2)
		}
		for _, line := range v {
			fmt.Println(line)
		}
		bad += len(v)
	}
	if bad > 0 {
		os.Exit(1)
	}
}
