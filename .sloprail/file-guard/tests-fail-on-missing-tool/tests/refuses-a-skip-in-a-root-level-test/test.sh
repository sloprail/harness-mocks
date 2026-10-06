#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only this rule's
# outcome is asserted. Proves a test file at the root of the repository is checked too (its directory is "."), and a package whose lookup is in a variable initializer is refused for a skip elsewhere.
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


git checkout -q -b root "$BASE"
printf 'package main\n\nimport (\n\t"os/exec"\n\t"testing"\n)\n\nfunc TestRoot(t *testing.T) {\n\tif _, err := exec.LookPath("zsh"); err != nil {\n\t\tt.Skip("no zsh")\n\t}\n}\n' > root_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a root-level test skips on a missing tool"
refuses "a skip in a root-level test" "$REASON"

# recovery: it fails instead, and the same range passes
git checkout -q -b rootfix "$BASE"
printf 'package main\n\nimport (\n\t"os/exec"\n\t"testing"\n)\n\nfunc TestRoot(t *testing.T) {\n\tif _, err := exec.LookPath("zsh"); err != nil {\n\t\tt.Fatalf("install zsh")\n\t}\n}\n' > root_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "the root-level test fails instead"
passes "a root-level test that fails on a missing tool"
