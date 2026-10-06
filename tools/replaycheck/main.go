// Command replaycheck type-checks the Go files that hold and read a mock's replay exception list
// (replay_allowlist_test.go, generated_replay_test.go), so the replay-exceptions-only-shrink
// file-guard judges their structure from the syntax tree and the type information, not from text.
//
//	replaycheck entries < replay_allowlist_test.go   one "key<TAB>rank" per entry of the notReplaying map
//	replaycheck check DIR CANONDIR                   the structure of the replay files of package dir DIR, against the canonical copies
//	replaycheck print FILE FUNC                      the function as it is compared (how a canonical copy is made)
//
// Exit 0: fine. Exit 1: a violation, one per line on stdout. Exit 2: the tool itself failed (stderr).
package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin *os.File, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: replaycheck entries < list.go | replaycheck check DIR CANONDIR | replaycheck print FILE FUNC")
		return 2
	}
	var violations []string
	var err error
	switch {
	case args[0] == "entries" && len(args) == 1:
		var lines []string
		lines, violations, err = entriesOf(stdin)
		if err == nil && len(violations) == 0 {
			for _, l := range lines {
				fmt.Fprintln(stdout, l)
			}
		}
	case args[0] == "check" && len(args) == 3:
		violations, err = checkDir(args[1], args[2])
	case args[0] == "print" && len(args) == 3:
		err = printNamed(args[1], args[2], stdout)
	default:
		fmt.Fprintln(stderr, "usage: replaycheck entries < list.go | replaycheck check DIR CANONDIR | replaycheck print FILE FUNC")
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "replaycheck:", err)
		return 2
	}
	for _, v := range violations {
		fmt.Fprintln(stdout, v)
	}
	if len(violations) > 0 {
		return 1
	}
	return 0
}
