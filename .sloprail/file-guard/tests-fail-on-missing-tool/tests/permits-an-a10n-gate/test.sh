#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only this rule's
# outcome is asserted. Proves the permit of an explicit opt-in gate (A10N_*_TEST) beside a lookup that fails the test, and of a skip in a test that looks nothing up.
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

branch_with gate "${HEAD_}func TestTool(t *testing.T) {
	_, err := exec.LookPath(\"zsh\")
	if err != nil {
		t.Fatalf(\"install zsh\")
	}
	if os.Getenv(\"A10N_REAL_CLAUDE_SUBAGENT_TEST\") != \"1\" {
		t.Skip(\"set A10N_REAL_CLAUDE_SUBAGENT_TEST=1\")
	}
}"
passes "an A10N_*_TEST gate beside a failing lookup"

branch_with short "${HEAD_}func TestTool(t *testing.T) {
	if testing.Short() {
		t.Skip(\"short\")
	}
}"
passes "a skip in a test that looks nothing up"
