#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only this rule's
# outcome is asserted. Proves a method value of Skip and a helper that skips are refused, and a gate on its own passes.
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

branch_with value "${HEAD_}func TestTool(t *testing.T) {
	skip := t.Skip
	if _, err := exec.LookPath(\"zsh\"); err != nil {
		skip(\"no zsh\")
	}
}"
refuses "a method value of Skip" "$REASON"

branch_with helper "${HEAD_}func need(t *testing.T) {
	if _, err := exec.LookPath(\"zsh\"); err != nil {
		t.Skip(\"no zsh\")
	}
}

func TestTool(t *testing.T) { need(t) }"
refuses "a helper that skips" "$REASON"

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
