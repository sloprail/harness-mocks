#!/usr/bin/env bash
set -euo pipefail

# adr-well-formed: an ADR.md that changed is a decision rules can enforce and people can apply; one judge call reviews
# every ADR whose ADR.md changed. The judge is a mock (SR_CHECKS_JUDGE_MOCKS): a Decision bullet that names no
# place where it applies fails, naming the ADR and the bullet; it asks nothing else (not a mechanism, not a rewording). The CI path, no agent turn: `sr-checks run` judges
# committed ranges with the project's rules (only this rule runs); its outcome is asserted on the FileGuardChecked
# events, one scenario per commit (a verdict is cached by content):
#   a sound ADR                                       -> passed
#   a bullet that names no place                      -> refused with the judge's reason
#   the bullet names its place (recovery)             -> passed
#   the user's own words, a hedge that names its place -> passed
#   an ADR deleted, a folder name outside the match, a file other than ADR.md -> not judged (the match's edge)
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=adr-well-formed
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
# only this rule runs: the others are switched off for the case
{ echo 'disabled:'; echo '  - sloprail/gate/ci-verify-required'; echo '  - sloprail/file-guard/rule-tests-pass'
  for d in .sloprail/file-guard/*/ .sloprail/gate/*/; do n="$(basename "$(dirname "$d")")/$(basename "$d")"; [ "$n" = "file-guard/$RULE" ] || echo "  - $n"; done; } >.sloprail/config.yaml
c() { git add -A && git commit -q -m "$1"; }
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/adr-well-formed/decision-is-sound":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
run_rule() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-well-formed")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-well-formed" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_judged() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-well-formed")] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}
# adr FOLDER BULLET — an ADR.md whose one Decision bullet is BULLET
adr() {
  mkdir -p "adr/$1"
  printf -- '---\nconcern: c\nsloprails: [file-guard/adr-well-formed]\n---\n## Concern\nc\n## Decision\n- %s\n' "$2" >"adr/$1/ADR.md"
}

adr base "code under \`internal/\` never says FORBIDDEN"
c base
BASE=$(git rev-parse HEAD)

# a sound ADR: judged and passed
git checkout -q -b sound "$BASE"
adr sound "every handler under \`internal/handlers\` returns an error value"; c "a sound ADR"
run_rule
expect_passed "a sound ADR"

# a bullet that names no place: refused with the judge's reason; naming it, the same range passes
git checkout -q -b vague "$BASE"
adr vague "handlers return an error value"; c "an ADR that names no place"
run_rule
expect_refused "a bullet that names no place" "a Decision bullet does not say where it applies: adr/vague names no place in 'handlers return an error value'"
adr vague "handlers under \`internal/handlers\` return an error value"; c "the bullet names where it applies"
run_rule
expect_passed "the bullet names where it applies"

# the user's own words stay as they are: a hedge, with no mechanism and no check, is not this judge's to refuse
git checkout -q -b words "$BASE"
adr words "we prefer small handlers under \`internal/handlers\`"; c "the user's words, a hedge that names its place"
run_rule
expect_passed "the user's own words, short and permissive"

# the match's edge: a deleted ADR, a folder name outside ^adr/[a-z0-9-]+/ADR.md, and another file of an ADR folder
git checkout -q -b deleted "$BASE"
git rm -q -r adr/base; c "the ADR is deleted"
run_rule
expect_not_judged "a deleted ADR"
git checkout -q -b outside "$BASE"
adr Odd_Name "handlers return an error value"
printf 'no place here\n' >adr/base/notes.md; c "a badly named ADR folder and a note beside an ADR"
run_rule
expect_not_judged "a folder name outside the match and a file other than ADR.md"
