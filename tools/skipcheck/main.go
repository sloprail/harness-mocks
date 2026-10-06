// Command skipcheck refuses a Go test that skips because a tool is missing. It type-checks the packages of
// the directories it is given (a package and its _test package together) and, when any code of the package
// (a function, a variable initializer, TestMain) names os/exec.LookPath, resolved by types so an alias or a
// dot import counts, reports every Skip, Skipf or SkipNow in it: a call, a method value, a generic, interface
// or type-parameter method. The one exemption is a skip directly under
// `if os.Getenv("A10N_<NAME>_TEST") <op> <constant>`: an explicit opt-in gate.
//
// Only a lookup inside the test package counts, not one through another package (the user's words: "a test
// package that looks up an external tool"), and only a named os/exec.LookPath counts: exec.Command is not
// treated as a lookup.
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
