package main

import (
	"strings"
	"testing"
)

const relocSrc = "package hooks\n\nimport \"github.com/x/mod/a/b\"\n\n// Fire runs.\nfunc Fire() { b.Do() }\n\n// note\n"

func relocated(body string) string {
	s := strings.Replace(relocSrc, "package hooks", "package hookrun", 1)
	s = strings.Replace(s, "github.com/x/mod/a/b", "github.com/x/mod/internal/b", 1)
	return strings.Replace(s, "b.Do()", body, 1)
}

func relocRepo(t *testing.T, after string) (*testRepo, string) {
	r := newTestRepo(t)
	base := r.commit("base", map[string]string{"old/hooks/h.go": relocSrc, "old/hooks/h_test.go": "package hooks\n\nfunc TestX() {}\n"})
	r.commit("relocate", map[string]string{
		"old/hooks/h.go": "", "old/hooks/h_test.go": "",
		"internal/hookrun/h.go": after, "internal/hookrun/h_test.go": "package hookrun\n\nfunc TestX() {}\n"})
	return r, base
}

func TestRename(t *testing.T) {
	const flag = "old/hooks=internal/hookrun"
	cases := []struct {
		name   string
		body   string
		args   []string
		want   int
		expect string
	}{
		{"pure relocation with --rename passes", "b.Do()", []string{"--rename", flag}, 0, "ok old/hooks -> internal/hookrun ("},
		{"same relocation without --rename fails", "b.Do()", nil, 1, "moved across directories"},
		{"relocation that edits a body fails", "b.Do2()", []string{"--rename", flag}, 1, "b.Do2()"},
		{"wrong --rename pairing fails", "b.Do()", []string{"--rename", "old/hooks=elsewhere"}, 1, "VIOLATION"},
		{"malformed --rename is a usage error", "b.Do()", []string{"--rename", "old/hooks"}, 2, "--rename"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, base := relocRepo(t, relocated(c.body))
			code, out := r.runTool(append([]string{"--base", base}, c.args...)...)
			if code != c.want || !strings.Contains(out, c.expect) {
				t.Fatalf("exit %d (want %d), want %q:\n%s", code, c.want, c.expect, out)
			}
		})
	}
}

func TestRenameKeepsPackageNameWhenLastElementIsUnchanged(t *testing.T) {
	r := newTestRepo(t)
	base := r.commit("base", map[string]string{"a/p/x.go": "package p\n\nfunc F() {}\n"})
	r.commit("move dir", map[string]string{"a/p/x.go": "", "b/p/x.go": "package q\n\nfunc F() {}\n"})
	if code, out := r.runTool("--base", base, "--rename", "a/p=b/p"); code != 1 {
		t.Fatalf("package clause may only change with the directory name; exit %d:\n%s", code, out)
	}
}
