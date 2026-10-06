#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only this rule's
# outcome is asserted. Proves a gate combined with the lookup error, and a skip many lines after the lookup, are refused (the checker resolves them by types, not by distance), and a gate on its own passes.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/tests-fail-on-missing-tool/tests/_setup.sh"
install_checker || exit 1
mkdir -p pkg
printf 'package pkg\n\nimport "testing"\n\nfunc TestTool(t *testing.T) {}\n' > pkg/tool_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "base"
BASE=$(git rev-parse HEAD)

HEAD_='package pkg

import (
	"os"
	"os/exec"
	"testing"
)

var _ = os.Getenv
var _ = exec.Command

'
REASON="which reaches os/exec.LookPath: a test whose required tool is missing fails, it never skips"

branch_with gate_or_err "${HEAD_}func TestTool(t *testing.T) {
	_, err := exec.LookPath(\"zsh\")
	if os.Getenv(\"A10N_X_TEST\") != \"1\" || err != nil {
		t.Skip(\"x\")
	}
}"
refuses "a gate combined with the lookup error" "$REASON"

branch_with far "${HEAD_}func TestTool(t *testing.T) {
	_, err := exec.LookPath(\"zsh\")
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
		t.Skipf(\"no zsh\")
	}
}"
refuses "a skip many lines after the lookup" "$REASON"

# recovery: a gate on its own, and a failing lookup, and the same base passes
branch_with ok "${HEAD_}func TestTool(t *testing.T) {
	if os.Getenv(\"A10N_X_TEST\") != \"1\" {
		t.Skip(\"set A10N_X_TEST=1\")
	}
	if _, err := exec.LookPath(\"zsh\"); err != nil {
		t.Fatalf(\"install zsh\")
	}
}"
passes "a gate on its own and a failing lookup"
