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

# the rule's outcome over BASE..HEAD, from the events of sr-checks run
passes() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="tests-fail-on-missing-tool")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1 was not passed by tests-fail-on-missing-tool (sr-checks exit $ran)" >&2; exit 1; }
}
refuses() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
  jq -es --arg r "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="tests-fail-on-missing-tool" and .outcome=="refused" and (.reason|contains($r)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1 was not refused with its reason (sr-checks exit $ran)" >&2; exit 1; }
}

HEAD_='package pkg

import (
	"os"
	"os/exec"
	"testing"
)

var _ = os.Getenv
var _ = exec.Command

'
REASON="which reaches os/exec.LookPath ("

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

# the skip moved to the external test package of the same directory: still the package that looks the tool up
git checkout -q -b external "$BASE"
printf 'package pkg\n\nimport (\n\t"os/exec"\n\t"testing"\n)\n\nfunc TestTool(t *testing.T) {\n\tif _, err := exec.LookPath("zsh"); err != nil {\n\t\tt.Fatalf("install zsh")\n\t}\n}\n' > pkg/tool_test.go
printf 'package pkg_test\n\nimport "testing"\n\nfunc TestOther(t *testing.T) { t.Skip("no zsh") }\n' > pkg/other_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a skip in the external test package"
refuses "a skip in the external test package" "$REASON"

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
