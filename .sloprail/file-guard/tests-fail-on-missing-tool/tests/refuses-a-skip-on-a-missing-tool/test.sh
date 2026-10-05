#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the refusal of a t.Skip after a failed exec.LookPath, and its recovery to t.Fatalf.
git init -q .
mkdir -p pkg
f=pkg/tool_test.go
printf 'package pkg\n\nimport "testing"\n\nfunc TestTool(t *testing.T) {}\n' > "$f"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "base"
BASE=$(git rev-parse HEAD)

# test_with BODY — the test file at the head carries BODY in TestTool
test_with() {
  printf 'package pkg\n\nimport (\n\t"os"\n\t"os/exec"\n\t"testing"\n)\n\nvar _ = os.Getenv\nvar _ = exec.Command\n\nfunc TestTool(t *testing.T) {\n%b}\n' "$1" > "$f"
  git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$2"
}
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
SKIP='\t_, err := exec.LookPath("zsh")\n\tif err != nil {\n\t\tt.Skip("zsh is not installed")\n\t}\n'
FAIL='\t_, err := exec.LookPath("zsh")\n\tif err != nil {\n\t\tt.Fatalf("zsh is not installed: install it")\n\t}\n'
OPTIN='\tif os.Getenv("A10N_REAL_CLAUDE_SUBAGENT_TEST") != "1" {\n\t\tt.Skip("set A10N_REAL_CLAUDE_SUBAGENT_TEST=1")\n\t}\n'

git checkout -q -b skips "$BASE"
test_with "$SKIP" "skip on a missing tool"
refuses "a skip on a missing tool" "a test whose required tool is missing fails, it never skips"
refuses "a skip on a missing tool, naming its file" "- pkg/tool_test.go:"

# the one-line form is refused too
git checkout -q -b oneline "$BASE"
test_with '\tif _, err := exec.LookPath("zsh"); err != nil { t.SkipNow() }\n' "one-line skip"
refuses "a one-line SkipNow after LookPath" "a test whose required tool is missing fails"

# recovery: the test fails instead, and the same range from the same base passes
test_with "$FAIL" "fail on a missing tool"
passes "a fail on a missing tool"
