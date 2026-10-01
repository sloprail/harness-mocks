package main

import (
	"strings"
	"testing"
)

const pkgA = "package p\n\n// Package p does things.\n\nimport \"fmt\"\n\n// Foo prints.\nfunc Foo() {\n\tfmt.Println(\"foo\") // trailing\n}\n\n// free-floating note\n\n// Bar returns one.\n//\n//go:noinline\nfunc Bar() int { return 1 }\n"

const (
	fooDecl = "// Foo prints.\nfunc Foo() {\n\tfmt.Println(\"foo\") // trailing\n}\n"
	barDecl = "// Bar returns one.\n//\n//go:noinline\nfunc Bar() int { return 1 }\n"
)

func split(a, b string) map[string]string {
	return map[string]string{
		"p/a.go": "package p\n\n// Package p does things.\n\nimport \"fmt\"\n\n" + a + "\n// free-floating note\n",
		"p/b.go": "package p\n\n" + b,
	}
}

func TestVerdicts(t *testing.T) {
	cases := []struct {
		name   string
		second map[string]string
		want   int
		expect string
	}{
		{"pure move passes", split(fooDecl, barDecl), 0, "ok p ("},
		{"edited line in moved func fails", split(strings.Replace(fooDecl, `"foo"`, `"fo"`, 1), barDecl), 1, `fmt.Println("fo")`},
		{"dropped doc comment fails", split(fooDecl, strings.Replace(barDecl, "// Bar returns one.\n//\n", "", 1)), 1, "VIOLATION p"},
		{"dropped free comment fails", map[string]string{
			"p/a.go": "package p\n\n// Package p does things.\n\nimport \"fmt\"\n\n" + fooDecl,
			"p/b.go": "package p\n\n" + barDecl}, 1, "free-floating note"},
		{"dropped trailing comment fails", split(strings.Replace(fooDecl, " // trailing", "", 1), barDecl), 1, "// trailing"},
		{"renamed func fails", split(fooDecl, strings.Replace(barDecl, "Bar", "Baz", 2)), 1, "func Baz"},
		{"changed import only passes", split(fooDecl, barDecl), 0, "ok p ("},
		{"move to another package fails", map[string]string{
			"p/a.go": "package p\n\n// Package p does things.\n\nimport \"fmt\"\n\n" + fooDecl + "\n// free-floating note\n",
			"p/b.go": "",
			"q/b.go": "package q\n\n" + barDecl}, 1, "moved across directories"},
		{"changed non-go file fails", map[string]string{"README.md": "changed\n"}, 1, "README.md"},
		{"package rename fails", map[string]string{"p/a.go": strings.Replace(pkgA, "package p", "package q", 1)}, 1, "VIOLATION p"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newTestRepo(t)
			base := r.commit("base", map[string]string{"p/a.go": pkgA, "README.md": "x\n"})
			r.commit("second", c.second)
			code, out := r.runTool("--base", base)
			if code != c.want || !strings.Contains(out, c.expect) {
				t.Fatalf("exit %d (want %d), want output containing %q:\n%s", code, c.want, c.expect, out)
			}
		})
	}
}

func TestImportEditsAreIgnored(t *testing.T) {
	r := newTestRepo(t)
	base := r.commit("base", map[string]string{"p/a.go": pkgA})
	// Same declarations, imports rewritten (fmt import is now unused in a.go
	// but the tool is syntactic, exactly like a real move that adds imports).
	r.commit("second", map[string]string{"p/a.go": strings.Replace(pkgA, "import \"fmt\"", "import (\n\t\"fmt\"\n\t\"os\"\n)", 1)})
	if code, out := r.runTool("--base", base); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestEachCatchesBadMiddleCommit(t *testing.T) {
	r := newTestRepo(t)
	base := r.commit("base", map[string]string{"p/a.go": pkgA})
	r.commit("bad", map[string]string{"p/a.go": strings.Replace(pkgA, "return 1", "return 2", 1)})
	r.commit("revert", map[string]string{"p/a.go": pkgA})
	if code, out := r.runTool("--base", base); code != 0 {
		t.Fatalf("range check should pass (net no change), exit %d:\n%s", code, out)
	}
	code, out := r.runTool("--base", base, "--each")
	if code != 1 || !strings.Contains(out, "return 2") {
		t.Fatalf("--each must fail on the middle commit, exit %d:\n%s", code, out)
	}
}

func TestUsageAndGitErrorsFailClosed(t *testing.T) {
	r := newTestRepo(t)
	r.commit("base", map[string]string{"p/a.go": pkgA})
	for _, args := range [][]string{{}, {"--base", "nosuchrev"}, {"--base", "HEAD", "--head", "nosuchrev"}, {"--bogus"}} {
		if code, out := r.runTool(args...); code != 2 {
			t.Errorf("%v: exit %d, want 2:\n%s", args, code, out)
		}
	}
	r.commit("broken", map[string]string{"p/a.go": "package p\nfunc {"})
	if code, out := r.runTool("--base", "HEAD~1"); code != 2 {
		t.Errorf("syntax error: exit %d, want 2:\n%s", code, out)
	}
}
