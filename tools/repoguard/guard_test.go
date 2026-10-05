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

// globalGitConfigWrites reports the commands in body that write the global or system git config,
// with the line each starts on. Reads (--get*, --list, -l, --show-origin) are fine. A command
// spread over lines (a shell backslash continuation, a Go call split after "," or "(") is judged
// as one; comment lines are prose and never join anything.
func globalGitConfigWrites(body string) []string {
	var out []string
	seen := map[string]bool{}
	check := func(cmd string, line int) {
		// each command of a line on its own: a read-only one beside a write must not exempt it
		for _, seg := range segmentRe.Split(cmd, -1) {
			if !strings.Contains(seg, "config") || readOnlyConfig.MatchString(seg) || !writesMachineConfig(seg) {
				continue
			}
			if shellGitConfig.MatchString(seg) || goGitConfig.MatchString(seg) {
				hit := strings.TrimSpace(seg) + " (line " + strconv.Itoa(line) + ")"
				if !seen[hit] {
					seen[hit] = true
					out = append(out, hit)
				}
			}
		}
	}
	cur, start := "", 0
	for i, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") {
			if cur != "" {
				check(cur, start)
				cur = ""
			}
			continue
		}
		if cur == "" {
			start = i + 1
		}
		// each physical line on its own too: a read-only command elsewhere in a joined chunk
		// (a table of commands) must not exempt a write on this one.
		check(t, i+1)
		cur += " " + strings.TrimSuffix(t, `\`)
		// a shell continuation always joins; a Go call split after "," or "(" joins only while
		// a call is open, so a table or slice literal is not one command.
		if strings.HasSuffix(t, `\`) ||
			((openCall(cur) || openBrace(cur)) &&
				(strings.HasSuffix(t, ",") || strings.HasSuffix(t, "(") || strings.HasSuffix(t, "{"))) {
			continue
		}
		check(cur, start)
		cur = ""
	}
	if cur != "" {
		check(cur, start)
	}
	return out
}

func openCall(s string) bool  { return strings.Count(s, "(") > strings.Count(s, ")") }
func openBrace(s string) bool { return strings.Count(s, "{") > strings.Count(s, "}") }

// writesMachineConfig: --global / --system, or --file/-f naming a config under a home directory
// (the XDG ~/.config/git/config included, a repository's own .git/config not).
func writesMachineConfig(cmd string) bool {
	if globalScope.MatchString(cmd) {
		return true
	}
	for _, m := range homeFile.FindAllString(cmd, -1) {
		if !strings.Contains(m, ".git/config") {
			return true
		}
	}
	return false
}

var (
	globalScope    = regexp.MustCompile(`--(global|system)\b`)
	homeFile       = regexp.MustCompile(`(--file|-f)[ =]+"?(~|\$HOME|\$\{HOME\}|/Users/|/home/)[^ ]*config\b`)
	readOnlyConfig = regexp.MustCompile(`--(get|get-all|get-regexp|list|show-origin)\b|\s-l\b`)
	shellGitConfig = regexp.MustCompile(`\bgit\b[^|;&]*\bconfig\b`)
	goGitConfig    = regexp.MustCompile(`"config"\s*,`)
	segmentRe      = regexp.MustCompile(`&&|\|\||;|\|`)
)

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
		if os.IsNotExist(err) {
			continue
		}
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
		"git config --global user.name x && git config --list",
		"git config --global user.name x; git config --global --list",
		"args := []string{\n\"config\",\n\"--global\",\n\"user.name\", \"x\",\n}",
		"x(\n\"git config --global --get a\",\n\"git config --global user.name x\")",
		"[]string{\n\"git config --list\",\n\"git config --global user.name x\",\n}",
		"git config --file $HOME/.config/git/config user.email x",
		"// setup (\nexec.Command(\"git\",\"config\",\"--global\",\"a\")",
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
