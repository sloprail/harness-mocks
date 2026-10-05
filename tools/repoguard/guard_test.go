// Package repoguard holds checks over this repository's own files.
package repoguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Nothing a test, a CI step, a script or a Makefile target runs may write the MACHINE's git
// configuration (--global / --system). On a CI runner that is harmless; copied onto a developer's
// machine ("run it exactly as CI does") it rewrites their real ~/.gitconfig, and every later
// commit they make carries the test identity (commits of a harness-mocks PR were authored
// "local test <ci-local@sloprail.invalid>" this way). A test identity is given as environment
// (GIT_AUTHOR_* / GIT_COMMITTER_*, see claude-mock/Makefile) or `git -c`, never as global config.

var (
	globalScope    = regexp.MustCompile(`--(global|system)\b|(--file|-f)[ =]+"?(~|\$HOME|\$\{HOME\}|/Users/|/home/)[^ ]*gitconfig`)
	readOnlyConfig = regexp.MustCompile(`--(get|get-all|get-regexp|list|show-origin)\b|\s-l\b`)
	shellGitConfig = regexp.MustCompile(`\bgit\b[^|;&]*\bconfig\b`)
	goGitConfig    = regexp.MustCompile(`"config"\s*,`)
)

// joined folds a backslash continuation and a Go call split after "," or "(" into one line, so a
// command spread over several lines is matched like one on a single line.
var joinRe = regexp.MustCompile(`\\\n\s*|[,(]\n\s*`)

func globalGitConfigWrites(body string) []string {
	var out []string
	body = joinRe.ReplaceAllStringFunc(body, func(m string) string {
		return strings.TrimRight(m, " \t\n\\") + " "
	})
	for i, line := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") {
			continue
		}
		if !strings.Contains(line, "config") || !globalScope.MatchString(line) || readOnlyConfig.MatchString(line) {
			continue
		}
		if shellGitConfig.MatchString(line) || goGitConfig.MatchString(line) {
			out = append(out, strings.TrimSpace(line)+" (line "+strconv.Itoa(i+1)+")")
		}
	}
	return out
}

func TestNothingWritesTheMachinesGitConfig(t *testing.T) {
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(string(top))
	files, err := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard").Output()
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, rel := range strings.Split(strings.TrimSpace(string(files)), "\n") {
		if rel == "tools/repoguard/guard_test.go" {
			continue
		}
		switch {
		case strings.HasPrefix(rel, ".github/"), filepath.Base(rel) == "Makefile":
		case filepath.Ext(rel) == ".sh", filepath.Ext(rel) == ".bash", filepath.Ext(rel) == ".mk", filepath.Ext(rel) == ".go":
		default:
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, hit := range globalGitConfigWrites(string(body)) {
			t.Errorf("%s writes the machine's git config: %s\n\tgive tests an identity through the environment, never the global config", rel, hit)
		}
	}
	if scanned < 20 {
		t.Fatalf("scanned only %d files: the selection is wrong", scanned)
	}
}

func TestDetector(t *testing.T) {
	for _, line := range []string{
		"git config --global user.email ci@sloprail.invalid",
		"git config \\\n  --global user.name x",
		"exec.Command(\"git\", \"config\",\n\t\"--global\", \"user.email\")",
		"git config --file ~/.gitconfig user.email x",
		"git -C /x config --system core.autocrlf false",
		`exec.Command("git", "config", "--global", "user.email", "x")`,
	} {
		if len(globalGitConfigWrites(line)) == 0 {
			t.Errorf("not detected: %s", line)
		}
	}
	for _, line := range []string{
		"git config user.email t@t", "git config --global --list", "git config --global --get user.email",
		"# git config --global is never run",
	} {
		if hits := globalGitConfigWrites(line); len(hits) != 0 {
			t.Errorf("false positive on %q: %v", line, hits)
		}
	}
}
