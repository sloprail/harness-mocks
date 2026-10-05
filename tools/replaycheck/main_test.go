package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodList = `package e2e

var notReplaying = map[string]string{
	"run-a": "adapter: x",
	"run-b": "untriaged: y",
	"run-c": "flaky: z",
}
`

const goodGen = `package e2e

import (
	"strings"
	"testing"
)

const flakyRuns = 3

func replayUntilGreen(run func() (string, error), attempts int) (string, error) { return run() }

func TestGeneratedReplay(t *testing.T) {
	for name := range notReplaying {
		_ = name
	}
	reason, listed := notReplaying["x"]
	flaky := listed && strings.HasPrefix(reason, "flaky:")
	var diff string
	var err error
	run := func() (string, error) { return "", nil }
	if flaky {
		diff, err = replayUntilGreen(run, flakyRuns)
	}
	switch {
	case flaky && (err != nil || diff != ""):
		t.Errorf("never green in %d runs, see notReplaying", flakyRuns)
	}
}
`

// dirWith writes the list and the generated test (and any extra files) into a temp package dir.
func dirWith(t *testing.T, list, gen string, extra map[string]string) string {
	t.Helper()
	d := t.TempDir()
	files := map[string]string{listFile: list, genFile: gen}
	for k, v := range extra {
		files[k] = v
	}
	for name, body := range files {
		if body == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(d, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func violationsOf(t *testing.T, dir string) string {
	t.Helper()
	v, err := checkDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(v, "\n")
}

func TestTheGoodFilesPass(t *testing.T) {
	if v := violationsOf(t, dirWith(t, goodList, goodGen, nil)); v != "" {
		t.Fatalf("a good package was refused:\n%s", v)
	}
}

func TestTheRealPackagesPass(t *testing.T) {
	for _, d := range []string{"../../claude-mock/e2e/018_replay", "../../codex-mock/e2e/001_hooks"} {
		if _, err := os.Stat(d); err != nil {
			t.Skipf("%s is not in this tree", d)
		}
		if v := violationsOf(t, d); v != "" {
			t.Errorf("%s:\n%s", d, v)
		}
	}
}

func TestBypassesAreRefused(t *testing.T) {
	cases := []struct {
		name, gen, extraName, extra, want string
	}{
		{"a closure shadows flakyRuns", strings.Replace(goodGen, "diff, err = replayUntilGreen(run, flakyRuns)", "func(flakyRuns int) { diff, err = replayUntilGreen(run, flakyRuns) }(0)", 1), "", "", "must be the package-level const flakyRuns"},
		{"a local var shadows flakyRuns", strings.Replace(goodGen, "diff, err = replayUntilGreen(run, flakyRuns)", "var flakyRuns int; diff, err = replayUntilGreen(run, flakyRuns)", 1), "", "", "is declared again here"},
		{"a literal instead of flakyRuns", strings.Replace(goodGen, "replayUntilGreen(run, flakyRuns)", "replayUntilGreen(run, 1)", 1), "", "", "must be the package-level const flakyRuns"},
		{"flakyRuns is not 3", strings.Replace(goodGen, "const flakyRuns = 3", "const flakyRuns = 1", 1), "", "", "flakyRuns must be 3"},
		{"flakyRuns declared twice", goodGen + "\nvar flakyRuns = 5\n", "", "", "declared exactly once"},
		{"replayUntilGreen defined twice", goodGen, "other_test.go", "package e2e\n\nfunc replayUntilGreen(run func() (string, error), n int) (string, error) { return run() }\n", "defined exactly once"},
		{"the result is dropped", strings.Replace(goodGen, "diff, err = replayUntilGreen(run, flakyRuns)", "_, _ = replayUntilGreen(run, flakyRuns)", 1), "", "", "must be kept"},
		{"an init in the generated test writes", goodGen + "\nfunc init() { notReplaying[\"z\"] = \"flaky: z\" }\n", "", "", "may only read notReplaying"},
		{"a write inside a t.Errorf call", strings.Replace(goodGen, `t.Errorf("never green in %d runs, see notReplaying", flakyRuns)`, `t.Errorf("x", func() int { notReplaying["z"] = "flaky: z"; return 0 }())`, 1), "", "", "may only read notReplaying"},
		{"a write after a t.Errorf on one line", strings.Replace(goodGen, `t.Errorf("never green in %d runs, see notReplaying", flakyRuns)`, `t.Errorf("x", flakyRuns); notReplaying["z"] = "y"`, 1), "", "", "may only read notReplaying"},
		{"a delete", goodGen + "\nfunc g() { delete(notReplaying, \"a\") }\n", "", "", "may only read notReplaying"},
		{"an alias", goodGen + "\nfunc g() { m := notReplaying; m[\"z\"] = \"y\" }\n", "", "", "may only read notReplaying"},
		{"an address", goodGen + "\nfunc g() { p := &notReplaying; _ = p }\n", "", "", "may only read notReplaying"},
		{"passed as a value", goodGen + "\nfunc h(m map[string]string) {}\nfunc g() { h(notReplaying) }\n", "", "", "may only read notReplaying"},
		{"an index in a range target", goodGen + "\nfunc g() { for notReplaying[\"z\"] = range []string{\"y\"} {} }\n", "", "", "may only read notReplaying"},
		{"an increment through an index", goodGen + "\nfunc g() { notReplaying[\"z\"] += \"y\" }\n", "", "", "may only read notReplaying"},
		{"a shadowing local named like the list", goodGen + "\nfunc g() { notReplaying := map[string]string{}; notReplaying[\"z\"] = \"y\" }\n", "", "", "not the package's list"},
		{"another file names the list", goodGen, "init_test.go", "package e2e\n\nfunc init() { notReplaying[\"z\"] = \"flaky: z\" }\n", "is named outside"},
		{"the prefix test is gone", strings.Replace(goodGen, `strings.HasPrefix(reason, "flaky:")`, `strings.HasPrefix(reason, "adapter:")`, 1), "", "", `strings.HasPrefix(reason, "flaky:")`},
		{"the failing case is gone", strings.Replace(goodGen, `case flaky && (err != nil || diff != ""):`, `case false:`, 1), "", "", "a case `flaky &&"},
		{"no replayUntilGreen call", strings.Replace(goodGen, "diff, err = replayUntilGreen(run, flakyRuns)", "diff, err = run()", 1), "", "", "must run a flaky: entry through"},
	}
	for _, c := range cases {
		extra := map[string]string{}
		if c.extraName != "" {
			extra[c.extraName] = c.extra
		}
		v := violationsOf(t, dirWith(t, goodList, c.gen, extra))
		if !strings.Contains(v, c.want) {
			t.Errorf("%s: wanted a violation saying %q, got:\n%s", c.name, c.want, v)
		}
	}
}

func TestReadsAreAllowed(t *testing.T) {
	gen := goodGen + "\nfunc g() { _, ok := notReplaying[\"a\"]; _ = ok; v := notReplaying[\"b\"]; _ = v }\n"
	if v := violationsOf(t, dirWith(t, goodList, gen, nil)); v != "" {
		t.Fatalf("reads were refused:\n%s", v)
	}
}

func TestTheListFileIsGoneWhileTheGeneratedTestReadsIt(t *testing.T) {
	v := violationsOf(t, dirWith(t, "", goodGen, nil))
	if !strings.Contains(v, "is gone or renamed") {
		t.Fatalf("got:\n%s", v)
	}
}

func TestEntriesDecodeWhatGoDecodes(t *testing.T) {
	cases := []struct{ name, list, want, violation string }{
		{"plain", goodList, "run-a\t2\nrun-b\t1\nrun-c\t0\n", ""},
		{"an escaped category", "package e2e\n\nvar notReplaying = map[string]string{\"a\": \"\\u0066laky: x\"}\n", "a\t0\n", ""},
		{"a raw string", "package e2e\n\nvar notReplaying = map[string]string{\"a\": `flaky: x`}\n", "a\t0\n", ""},
		{"a one-line map with a comment", "package e2e\n\nvar notReplaying = map[string]string{\"a\": \"untriaged: x\", /* c */ \"b\": \"adapter: y\"} // end\n", "a\t1\nb\t2\n", ""},
		{"a var group", "package e2e\n\nvar (\n\tnotReplaying = map[string]string{\"a\": \"x\"}\n)\n", "a\t2\n", ""},
		{"an empty map", "package e2e\n\nvar notReplaying = map[string]string{}\n", "", ""},
		{"a second declaration", "package e2e\n\nvar notReplaying = map[string]string{}\nvar notReplaying = map[string]string{}\n", "", "exactly once"},
		{"an init writing", "package e2e\n\nvar notReplaying = map[string]string{}\n\nfunc init() { notReplaying[\"a\"] = \"flaky: x\" }\n", "", "named 2 times"},
		{"a concatenated reason", "package e2e\n\nvar notReplaying = map[string]string{\"a\": \"flaky\" + \": x\"}\n", "", "not a pair of string literals"},
		{"a built map", "package e2e\n\nvar notReplaying = build()\n\nfunc build() map[string]string { return nil }\n", "", "must be a map composite literal"},
		{"a duplicate key", "package e2e\n\nvar notReplaying = map[string]string{\"a\": \"x\", \"a\": \"y\"}\n", "", "twice"},
	}
	for _, c := range cases {
		lines, viol, err := entriesOf(strings.NewReader(c.list))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got := strings.Join(lines, "\n")
		if got != "" {
			got += "\n"
		}
		if c.violation != "" {
			if !strings.Contains(strings.Join(viol, "\n"), c.violation) {
				t.Errorf("%s: wanted a violation saying %q, got %v", c.name, c.violation, viol)
			}
			continue
		}
		if len(viol) > 0 || got != c.want {
			t.Errorf("%s: want %q, got %q (violations %v)", c.name, c.want, got, viol)
		}
	}
}
