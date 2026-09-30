package main

import "testing"

const twoInits = "package p\n\nfunc init() { a() }\n\nfunc a() {}\n"

func TestFileSensitiveMovesFail(t *testing.T) {
	cases := []struct {
		name          string
		before, after map[string]string
	}{
		{"init moved to another file",
			map[string]string{"p/a.go": twoInits, "p/b.go": "package p\n"},
			map[string]string{"p/a.go": "package p\n\nfunc a() {}\n", "p/b.go": "package p\n\nfunc init() { a() }\n"}},
		{"decl moved out of a build-constrained file",
			map[string]string{"p/a.go": "//go:build linux\n\npackage p\n\nfunc a() {}\n", "p/b.go": "package p\n"},
			map[string]string{"p/a.go": "//go:build linux\n\npackage p\n", "p/b.go": "package p\n\nfunc a() {}\n"}},
		{"decl moved out of a GOOS-suffixed file",
			map[string]string{"p/a_linux.go": "package p\n\nfunc a() {}\n", "p/b.go": "package p\n"},
			map[string]string{"p/a_linux.go": "package p\n", "p/b.go": "package p\n\nfunc a() {}\n"}},
		{"decl moved from a _test file into production code",
			map[string]string{"p/a_test.go": "package p\n\nfunc a() {}\n", "p/b.go": "package p\n"},
			map[string]string{"p/a_test.go": "package p\n", "p/b.go": "package p\n\nfunc a() {}\n"}},
		{"go:generate comment moved to another file",
			map[string]string{"p/a.go": "package p\n\n//go:generate echo hi\n\nfunc a() {}\n", "p/b.go": "package p\n"},
			map[string]string{"p/a.go": "package p\n\nfunc a() {}\n", "p/b.go": "package p\n\n//go:generate echo hi\n"}},
		{"go directive detached from its declaration",
			map[string]string{"p/a.go": "package p\n\n//go:noinline\nfunc a() {}\n"},
			map[string]string{"p/a.go": "package p\n\n//go:noinline\n\nfunc a() {}\n"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newTestRepo(t)
			base := r.commit("base", c.before)
			r.commit("move", c.after)
			if code, out := r.runTool("--base", base); code != 1 {
				t.Fatalf("exit %d, want 1:\n%s", code, out)
			}
		})
	}
}

func TestInitStayingPutPasses(t *testing.T) {
	r := newTestRepo(t)
	base := r.commit("base", map[string]string{"p/a.go": twoInits, "p/b.go": "package p\n"})
	r.commit("move a", map[string]string{"p/a.go": "package p\n\nfunc init() { a() }\n", "p/b.go": "package p\n\nfunc a() {}\n"})
	if code, out := r.runTool("--base", base); code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}
