package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const canonDir = "../../.sloprail/file-guard/replay-exceptions-only-shrink/canonical"

const goodList = `package e2e

var notReplaying = map[string]string{
	"run-a": "adapter: x",
	"run-b": "untriaged: y",
	"run-c": "flaky: z",
}
`

// canonFile is a canonical copy of the rule's folder.
func canonFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(canonDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// goodGen is a generated replay test the rule accepts: the canonical copies in a package with the const.
func goodGen(t *testing.T) string {
	t.Helper()
	return "package e2e\n\nconst flakyRuns = 3\n\n" + canonFile(t, "replay_until_green.go.txt") + "\n" + canonFile(t, "test_generated_replay.claude.go.txt")
}

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
	v, err := checkDir(dir, canonDir)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(v, "\n")
}

func TestTheGoodFilesPass(t *testing.T) {
	if v := violationsOf(t, dirWith(t, goodList, goodGen(t), nil)); v != "" {
		t.Fatalf("a good package was refused:\n%s", v)
	}
}

func TestCommentsAndSpacingDoNotMatter(t *testing.T) {
	gen := strings.Replace(goodGen(t), "diff, err := run()", "// a comment\n\tdiff, err := run()   /* and another */", 1)
	if v := violationsOf(t, dirWith(t, goodList, gen, nil)); v != "" {
		t.Fatalf("a comment changed what was compared:\n%s", v)
	}
}

func TestTheRealPackagesPass(t *testing.T) {
	for _, d := range []string{"../../claude-mock/e2e/018_replay", "../../codex-mock/e2e/002_replay", "../../cursor-mock/e2e/003_replay"} {
		if _, err := os.Stat(d); err != nil {
			t.Fatalf("%s is not in this tree: every mock's replay package is pinned", d)
		}
		if v := violationsOf(t, d); v != "" {
			t.Errorf("%s:\n%s", d, v)
		}
	}
}

func TestReadsAreAllowed(t *testing.T) {
	gen := goodGen(t) + "\nfunc g() { _, ok := notReplaying[\"a\"]; _ = ok; v := notReplaying[\"b\"]; _ = v; for k := range (notReplaying) { _ = k } }\n"
	if v := violationsOf(t, dirWith(t, goodList, gen, nil)); v != "" {
		t.Fatalf("reads were refused:\n%s", v)
	}
}

func TestTheListFileIsGoneWhileTheGeneratedTestReadsIt(t *testing.T) {
	v := violationsOf(t, dirWith(t, "", goodGen(t), nil))
	if !strings.Contains(v, "is gone or renamed") {
		t.Fatalf("got:\n%s", v)
	}
}

func TestAMissingCanonicalCopyIsAnErrorNotAPass(t *testing.T) {
	if _, err := checkDir(dirWith(t, goodList, goodGen(t), nil), t.TempDir()); err == nil {
		t.Fatal("a missing canonical copy passed")
	}
}

// An entry says whether its reason starts with a category the user named.
func TestEntriesSayWhetherTheReasonIsKnown(t *testing.T) {
	src := "package e2e\n\nvar notReplaying = map[string]string{\n\t\"a\": \"adapter: x\",\n\t\"b\": \"mock gap: x\",\n\t\"c\": \"race: x\",\n\t\"d\": \"flaky: x\",\n}\n"
	lines, violations, err := entriesOf(strings.NewReader(src))
	if err != nil || len(violations) > 0 {
		t.Fatalf("%v %v", err, violations)
	}
	want := []string{"a\t2\t1", "b\t2\t1", "c\t2\t0", "d\t0\t1"}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q, want %q", lines, want)
	}
}
