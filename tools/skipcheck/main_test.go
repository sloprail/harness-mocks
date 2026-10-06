package main

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const head = `package pkg

import (
	"os"
	"os/exec"
	"testing"
)

var _ = os.Getenv
var _ = exec.Command

`

func violations(t *testing.T, body string, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"a_test.go": body}
	for k, v := range extra {
		files[k] = v
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fset := token.NewFileSet()
	v, err := checkDir(fset, newImporter(fset), dir)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(v, "\n")
}

func TestSkipAfterAMissingToolIsRefused(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"a skip after a failed lookup", head + `func TestX(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("no zsh")
	}
}`, "(*testing.T).Skip"},
		{"a gate combined with the lookup error", head + `func TestX(t *testing.T) {
	_, err := exec.LookPath("zsh")
	if os.Getenv("A10N_X_TEST") != "1" || err != nil {
		t.Skip("x")
	}
}`, "(*testing.T).Skip"},
		{"a skip far after the lookup", head + `func TestX(t *testing.T) {
	_, err := exec.LookPath("zsh")
	_ = err
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
	f := 6
	g := 7
	_, _, _, _, _, _, _ = a, b, c, d, e, f, g
	if err != nil {
		t.Skipf("no zsh")
	}
}`, "(*testing.T).Skipf"},
		{"an alias of os/exec", `package pkg

import (
	e "os/exec"
	"testing"
)

func TestX(t *testing.T) {
	if _, err := e.LookPath("zsh"); err != nil {
		t.SkipNow()
	}
}`, "(*testing.T).SkipNow"},
		{"a dot import of os/exec", `package pkg

import (
	. "os/exec"
	"testing"
)

func TestX(t *testing.T) {
	if _, err := LookPath("zsh"); err != nil {
		t.Skip("x")
	}
}`, "(*testing.T).Skip"},
		{"a method value", head + `func TestX(t *testing.T) {
	skip := t.Skip
	if _, err := exec.LookPath("zsh"); err != nil {
		skip("no zsh")
	}
}`, "(*testing.T).Skip"},
		{"a helper that skips", head + `func need(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("no zsh")
	}
}

func TestX(t *testing.T) { need(t) }`, "(*testing.T).Skip"},
		{"a skipping helper called where the lookup is a helper's", head + `func has() bool { _, err := exec.LookPath("zsh"); return err == nil }

func skipIt(t *testing.T) { t.Skip("x") }

func TestX(t *testing.T) {
	if !has() {
		skipIt(t)
	}
}`, "(*testing.T).Skip"},
		{"a lookup in a package variable, the skip elsewhere", head + `var zsh, zshErr = exec.LookPath("zsh")

func TestX(t *testing.T) {
	if zshErr != nil || zsh == "" {
		t.Skip("no zsh")
	}
}`, "(*testing.T).Skip"},
		{"a lookup in TestMain, the skip in a test", head + `func TestMain(m *testing.M) {
	_, err := exec.LookPath("zsh")
	if err != nil {
		os.Setenv("NO_ZSH", "1")
	}
	os.Exit(m.Run())
}

func TestX(t *testing.T) {
	if os.Getenv("NO_ZSH") != "" {
		t.Skip("no zsh")
	}
}`, "(*testing.T).Skip"},
		{"an interface skip on testing.TB", head + `func skipper(tb testing.TB) {
	_, err := exec.LookPath("zsh")
	if err != nil {
		tb.Skip("no zsh")
	}
}`, "(*testing.T).Skip"},
		{"a gate in an else branch", head + `func TestX(t *testing.T) {
	_, err := exec.LookPath("zsh")
	if os.Getenv("A10N_X_TEST") == "" {
		_ = err
	} else {
		t.Skip("x")
	}
}`, "(*testing.T).Skip"},
		{"a gate that is not an A10N_*_TEST variable", head + `func TestX(t *testing.T) {
	_, err := exec.LookPath("zsh")
	_ = err
	if os.Getenv("SKIP_ZSH") != "" {
		t.Skip("x")
	}
}`, "(*testing.T).Skip"},
	}
	for _, c := range cases {
		if v := violations(t, c.body, nil); !strings.Contains(v, c.want) {
			t.Errorf("%s: wanted a violation saying %q, got %q", c.name, c.want, v)
		}
	}
}

func TestPermittedForms(t *testing.T) {
	cases := []struct{ name, body string }{
		{"a fail on a missing tool", head + `func TestX(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Fatalf("install zsh")
	}
}`},
		{"an opt-in gate beside a failing lookup", head + `func TestX(t *testing.T) {
	_, err := exec.LookPath("zsh")
	if err != nil {
		t.Fatalf("install zsh")
	}
	if os.Getenv("A10N_REAL_CLAUDE_SUBAGENT_TEST") != "1" {
		t.Skip("set A10N_REAL_CLAUDE_SUBAGENT_TEST=1")
	}
}`},
		{"a skip in a test that looks nothing up", head + `func TestX(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
}`},
	}
	for _, c := range cases {
		if v := violations(t, c.body, nil); v != "" {
			t.Errorf("%s was refused:\n%s", c.name, v)
		}
	}
}

func TestADirThatIsMissingOrEmptyIsAnErrorNotAPass(t *testing.T) {
	fset := token.NewFileSet()
	if _, err := checkDir(fset, newImporter(fset), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("a missing directory passed")
	}
	if _, err := checkDir(fset, newImporter(fset), t.TempDir()); err == nil {
		t.Error("a directory with no .go file passed")
	}
}
