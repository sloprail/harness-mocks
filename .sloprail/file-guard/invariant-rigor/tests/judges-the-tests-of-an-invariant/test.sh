#!/usr/bin/env bash
set -euo pipefail

# invariant-rigor: the tests marked // sr:proves <id>, together, rigorously prove the invariant. One subject per
# invariant touched: its spec file, a test proving it, or code upholding it changed. The judge is a mock
# (SR_CHECKS_JUDGE_MOCKS): a test with no t.Fatal asserts nothing and fails, naming the test. The CI path, no agent
# turn: `sr-checks run` judges committed ranges with the project's rules (only this rule runs); its outcome is
# asserted on the FileGuardChecked events, one scenario per commit (a verdict is cached by content):
#   a proving test emptied of its assertion          -> refused with the judge's reason naming the test
#   the assertion put back (recovery)                -> passed
#   the code upholding the invariant changed, tests asserting -> judged, passed
#   a second proving test that asserts nothing       -> refused (every test is held to the statement)
#   a change touching no invariant                   -> not judged
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=invariant-rigor
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/invariant-rigor/tests-prove-statement":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg r "$RULE" --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule==$r and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_judged() {   # LABEL
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}
# strong FILE — a test that proves invariant x and asserts; weak FILE — one that is marked but asserts nothing
strong() { printf 'package internal\n\n// sr:proves x\nfunc %s(t *testing.T) {\n\tif f(0) != 0 {\n\t\tt.Fatal("f(0)")\n\t}\n}\n' "$2" >"$1"; }
weak() { printf 'package internal\n\n// sr:proves x\nfunc %s(t *testing.T) {}\n' "$2" >"$1"; }

mkdir -p spec/invariants internal
printf 'statement: f of zero is zero\n' >spec/invariants/x.yaml
printf 'package internal\n\n// sr:invariant x\nfunc f(n int) int { return n }\n' >internal/x.go
strong internal/x_test.go TestX
echo base >README
c base
BASE=$(git rev-parse HEAD)

# a change touching no invariant is not judged
git checkout -q -b elsewhere "$BASE"
echo more >>README; c "the readme changes"
run_rule
expect_not_judged "a change touching no invariant"

# a proving test emptied of its assertion: refused with the judge's reason; the assertion back, the range passes
git checkout -q -b emptied "$BASE"
weak internal/x_test.go TestX; c "the test asserts nothing"
run_rule
expect_refused "a proving test that asserts nothing" "these tests assert nothing the statement says: internal/x_test.go"
strong internal/x_test.go TestX; printf '// reworded\n' >>internal/x_test.go; c "the assertion is back"
run_rule
expect_passed "the assertion restored"

# the code upholding the invariant changes, the test still asserts: judged and passed
git checkout -q -b impl "$BASE"
printf 'package internal\n\n// sr:invariant x\nfunc f(n int) int { return n * 1 }\n' >internal/x.go; c "the implementation changes"
run_rule
expect_passed "an implementation change under asserting tests"

# a second proving test that asserts nothing: every proving test is held to the statement
git checkout -q -b second "$BASE"
weak internal/x2_test.go TestX2; c "a second, empty proof"
run_rule
expect_refused "a second proving test that asserts nothing" "these tests assert nothing the statement says: internal/x2_test.go"
