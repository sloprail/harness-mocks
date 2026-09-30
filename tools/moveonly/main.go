// Command moveonly proves that a range of commits only moved Go declarations
// between files of the same package.
//
//	go run ./tools/moveonly --base <rev> [--head <rev>] [--each] [--repo <dir>]
//
// Exit status: 0 every package passes, 1 a violation, 2 usage or git error.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("moveonly", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "", "base revision (required)")
	head := fs.String("head", "HEAD", "head revision")
	each := fs.Bool("each", false, "check every commit in base..head against its own parent")
	dir := fs.String("repo", ".", "repository directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *base == "" || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: moveonly --base <rev> [--head <rev>] [--each] [--repo <dir>]")
		return 2
	}
	r := repo{*dir}
	pass, err := verify(r, *base, *head, *each, stdout)
	switch {
	case err != nil:
		fmt.Fprintln(stderr, "moveonly:", err)
		return 2
	case !pass:
		return 1
	}
	return 0
}

// verify runs the requested checks and reports whether all of them passed.
func verify(r repo, base, head string, each bool, w io.Writer) (bool, error) {
	b, err := r.resolve(base)
	if err != nil {
		return false, fmt.Errorf("cannot resolve base %q: %w", base, err)
	}
	h, err := r.resolve(head)
	if err != nil {
		return false, fmt.Errorf("cannot resolve head %q: %w", head, err)
	}
	if !each {
		o, err := checkPair(r, b, h)
		return err == nil && report(w, "", o), err
	}
	commits, err := r.commits(b, h)
	if err != nil {
		return false, err
	}
	pass := true
	for _, c := range commits {
		o, err := checkPair(r, c+"^", c)
		if err != nil {
			return false, err
		}
		if !report(w, c[:7]+" ", o) {
			pass = false
		}
	}
	return pass, nil
}
