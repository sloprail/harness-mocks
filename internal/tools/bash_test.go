package tools

import (
	"context"
	"testing"
)

func TestBashReportsOutputAndExit(t *testing.T) {
	env := []string{"PATH=/usr/bin:/bin", "WHO=world"}
	r := Bash(context.Background(), `echo "hello $WHO"; echo oops >&2; exit 4`, t.TempDir(), env)
	if r.ExitCode != 4 || r.Output != "hello world\noops\n" || r.Stdout != "hello world\n" || r.Stderr != "oops\n" || !r.Failed() {
		t.Fatalf("Bash = %+v", r)
	}
	if ok := Bash(context.Background(), "true", t.TempDir(), env); ok.Failed() {
		t.Fatalf("true failed: %+v", ok)
	}
}

func TestBashRunsInDirWithOnlyTheGivenEnvironment(t *testing.T) {
	dir := t.TempDir()
	r := Bash(context.Background(), `pwd -P; echo "${HOME:-unset}"`, dir, []string{"PATH=/usr/bin:/bin"})
	if r.ExitCode != 0 || r.Output[len(r.Output)-6:] != "unset\n" {
		t.Fatalf("Bash = %+v", r)
	}
}

func TestBashShellThatCannotStartIsAnExit(t *testing.T) {
	r := Bash(context.Background(), "true", "/nonexistent-dir-for-this-test", nil)
	if r.ExitCode != -1 || r.Output == "" {
		t.Fatalf("Bash = %+v, want exit -1 with the reason", r)
	}
}
