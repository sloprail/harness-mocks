package main

import (
	"strings"
	"testing"
)

// Each case changes the canonical code a little (or adds to the package) and must be refused.
func TestBypassesAreRefused(t *testing.T) {
	pin := "differs from the canonical copy"
	read := "may only read notReplaying"
	g := goodGen(t)
	cases := []struct {
		name, gen, extraName, extra, want string
	}{
		// the pinned functions
		{"the loop is changed", strings.Replace(g, "i < attempts &&", "i < 1 &&", 1), "", "", pin},
		{"the loop discards run()'s result", strings.Replace(g, "\t\tdiff, err = run()\n", "\t\t_, _ = run()\n", 1), "", "", pin},
		{"replayUntilGreen is a stub", strings.Replace(g, "diff, err := run()\n\tfor i := 1; i < attempts && (err != nil || diff != \"\"); i++ {\n\t\tdiff, err = run()\n\t}\n\treturn diff, err", "return run()", 1), "", "", pin},
		{"the failing case is changed", strings.Replace(g, `case flaky && (err != nil || diff != ""):`, `case flaky && false:`, 1), "", "", pin},
		{"an earlier case is always true", strings.Replace(g, "\tswitch {\n", "\tswitch {\n\tcase true:\n", 1), "", "", pin},
		{"the failing case is under a constant false", strings.Replace(strings.Replace(g, "\t\t\tswitch {\n", "\t\t\tif false { switch {\n", 1), "\t\t\t}\n\t\t})", "\t\t\t} }\n\t\t})", 1), "", "", pin},
		{"the test fails in a dead closure", strings.Replace(strings.Replace(g, "t.Errorf(\"flaky entry never", "_ = func() { t.Errorf(\"flaky entry never", 1), "flakyRuns, reason, err, diff)", "flakyRuns, reason, err, diff) }", 1), "", "", pin},
		{"the flaky define is always false", strings.Replace(g, `flaky := listed && strings.HasPrefix(reason, "flaky:")`, `flaky := false`, 1), "", "", pin},
		{"err is reassigned", strings.Replace(g, "\t\t\tvar unbuildable", "\t\t\terr = nil\n\t\t\tvar unbuildable", 1), "", "", pin},
		{"another *testing.T is used", strings.Replace(g, "func(t *testing.T) {\n\t\t\tt.Parallel()", "func(t *testing.T) {\n\t\t\tt := new(testing.T)\n\t\t\tt.Parallel()", 1), "", "", pin},
		{"the call uses another count", strings.Replace(g, "replayUntilGreen(run, flakyRuns)", "replayUntilGreen(run, 1)", 1), "", "", pin},
		{"a closure shadows flakyRuns", strings.Replace(g, "diff, err = replayUntilGreen(run, flakyRuns)", "func(flakyRuns int) { diff, err = replayUntilGreen(run, flakyRuns) }(0)", 1), "", "", pin},
		{"a local var shadows flakyRuns", strings.Replace(g, "diff, err = replayUntilGreen(run, flakyRuns)", "var flakyRuns int; diff, err = replayUntilGreen(run, flakyRuns)", 1), "", "", pin},
		{"the test is gone", strings.Replace(g, "func TestGeneratedReplay(", "func TestSomethingElse(", 1), "", "", "must be defined exactly once"},
		{"replayUntilGreen is defined twice", g, "other_test.go", "package e2e\n\nfunc replayUntilGreen(run func() (string, error), n int) (string, error) { return run() }\n", "must be defined exactly once"},
		// the constant
		{"flakyRuns is not 3", strings.Replace(g, "const flakyRuns = 3", "const flakyRuns = 1", 1), "", "", "flakyRuns must be 3"},
		{"flakyRuns declared twice", g + "\nvar flakyRuns = 5\n", "", "", "declared exactly once"},
		// the list is only read
		{"an init in the generated test writes", g + "\nfunc init() { notReplaying[\"z\"] = \"flaky: z\" }\n", "", "", read},
		{"a write inside a t.Errorf call", g + "\nfunc g() { t := 0; _ = t; println(func() int { notReplaying[\"z\"] = \"flaky: z\"; return 0 }()) }\n", "", "", read},
		{"a delete", g + "\nfunc g() { delete(notReplaying, \"a\") }\n", "", "", read},
		{"an alias", g + "\nfunc g() { m := notReplaying; m[\"z\"] = \"y\" }\n", "", "", read},
		{"an address", g + "\nfunc g() { p := &notReplaying; _ = p }\n", "", "", read},
		{"passed as a value", g + "\nfunc h(m map[string]string) {}\nfunc g() { h(notReplaying) }\n", "", "", read},
		{"an index in a range target", g + "\nfunc g() { for notReplaying[\"z\"] = range []string{\"y\"} {} }\n", "", "", read},
		{"an index appended to", g + "\nfunc g() { notReplaying[\"z\"] += \"y\" }\n", "", "", read},
		{"a shadowing local named like the list", g + "\nfunc g() { notReplaying := map[string]string{}; notReplaying[\"z\"] = \"y\" }\n", "", "", "not the package's list"},
		{"parenthesised index assigned", g + "\nfunc g() { (notReplaying[\"z\"]) = \"flaky:y\" }\n", "", "", read},
		{"parenthesised map indexed and assigned", g + "\nfunc g() { (notReplaying)[\"z\"] = \"y\" }\n", "", "", read},
		{"parenthesised index appended to", g + "\nfunc g() { (notReplaying[\"z\"]) += \"y\" }\n", "", "", read},
		{"parenthesised index as a range target", g + "\nfunc g() { for (notReplaying[\"z\"]) = range []string{\"y\"} {} }\n", "", "", read},
		{"parenthesised increment", g + "\nfunc g() { (notReplaying[\"z\"])++ }\n", "", "", read},
		{"another file names the list", g, "init_test.go", "package e2e\n\nfunc init() { notReplaying[\"z\"] = \"flaky: z\" }\n", "is named outside"},
	}
	for _, c := range cases {
		extra := map[string]string{}
		if c.extraName != "" {
			extra[c.extraName] = c.extra
		}
		if c.gen == g && c.extraName == "" {
			t.Errorf("%s: the case changes nothing", c.name)
			continue
		}
		v := violationsOf(t, dirWith(t, goodList, c.gen, extra))
		if !strings.Contains(v, c.want) {
			t.Errorf("%s: wanted a violation saying %q, got:\n%s", c.name, c.want, v)
		}
	}
}
