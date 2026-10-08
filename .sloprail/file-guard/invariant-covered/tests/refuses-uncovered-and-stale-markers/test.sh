#!/usr/bin/env bash
set -euo pipefail

# invariant-covered: every spec/invariants/<id>.yaml has code marked // sr:invariant <id> and a *_test.go marked
# // sr:proves <id>, and back: every marker names an invariant, and sr:proves sits in a test. The CI path, no agent
# turn: `sr-checks run` judges committed ranges with the project's rules (only this rule runs); its outcome is
# asserted on the FileGuardChecked events, one scenario per commit (a verdict is cached by content):
#   a covered invariant                          -> passed
#   its proving test deleted                     -> refused ("has no test"); the test back (recovery) -> passed
#   a marker naming no invariant, an sr:proves outside a test, a file name that is not kebab-case -> refused, each
#   a change the rule does not match             -> not judged
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=invariant-covered
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="invariant-covered")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="invariant-covered" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_judged() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="invariant-covered")] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}
impl() { mkdir -p internal; printf 'package internal\n\n// sr:invariant %s\nfunc %s() {}\n' "$1" "${1//-/_}" >"internal/$1.go"; }
proof() { mkdir -p internal; printf 'package internal\n\n// sr:proves %s\nfunc Test_%s() {}\n' "$1" "${1//-/_}" >"internal/$1_test.go"; }

mkdir -p spec/invariants
printf 'statement: it holds\n' >spec/invariants/holds.yaml
impl holds; proof holds
echo base >README
c base
BASE=$(git rev-parse HEAD)

# an untouched, covered invariant: a change elsewhere is not judged at all
git checkout -q -b elsewhere "$BASE"
echo more >>README; c "the readme changes"
run_rule
expect_not_judged "a change outside the match"

# the proving test deleted: refused for the missing test; it comes back and the same range passes
git checkout -q -b untested "$BASE"
git rm -q internal/holds_test.go; c "the proving test is deleted"
run_rule
expect_refused "a proving test deleted" "invariant 'holds' has no test: mark a test that proves it with // sr:proves holds"
# (in another file: a test deleted and restored byte for byte leaves nothing in the range to judge)
printf 'package internal\n\n// sr:proves holds\nfunc TestHoldsAgain() {}\n' >internal/holds_again_test.go; c "a test proves it again"
run_rule
expect_passed "the proving test restored"

# the implementation deleted
git checkout -q -b unimplemented "$BASE"
git rm -q internal/holds.go; c "the implementation is deleted"
run_rule
expect_refused "an implementation deleted" "invariant 'holds' has no implementation: mark the code that upholds it with // sr:invariant holds"

# a marker naming no invariant
git checkout -q -b ghost "$BASE"
impl ghost; c "code marked for an invariant nobody wrote"
run_rule
expect_refused "a marker naming no invariant" "internal/ghost.go: sr:invariant 'ghost' names no spec/invariants/ghost.yaml"
# recovery: the invariant is written, implemented and proven
printf 'statement: ghosts hold\n' >spec/invariants/ghost.yaml; proof ghost; c "the invariant is written and proven"
run_rule
expect_passed "the ghost invariant written and proven"

# a proves marker in non-test code
git checkout -q -b proves-in-code "$BASE"
printf 'package internal\n\n// sr:proves holds\nfunc notATest() {}\n' >internal/helper.go; c "a proves marker outside a test"
run_rule
expect_refused "an sr:proves outside a test" "internal/helper.go: sr:proves belongs on a test, in a *_test.go"

# a file name that is not kebab-case
git checkout -q -b badname "$BASE"
printf 'statement: it also holds\n' >spec/invariants/Also_Holds.yaml; c "an invariant with a bad file name"
run_rule
expect_refused "an invariant file name that is not kebab-case" "spec/invariants/Also_Holds.yaml: the file name must be kebab-case"
