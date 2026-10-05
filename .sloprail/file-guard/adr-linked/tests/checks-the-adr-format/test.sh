#!/usr/bin/env bash
set -euo pipefail

# adr-linked: every ADR is a kebab-case folder with an ADR.md that has ## Concern and ## Decision and links the
# rules that enforce it, each one existing in .sloprail/. The CI path, no agent turn: `sr-checks run` judges
# committed ranges with the project's rules (only this rule runs); its outcome is asserted on the
# FileGuardChecked events, one scenario per commit (a verdict is cached by content):
#   a well-formed ADR                                  -> passed
#   a folder name that is not kebab-case, no ## Decision, links no sloprail, a link to a rule that does not
#   exist                                              -> refused, each with its own reason
#   the link fixed (recovery)                          -> passed
#   a linked rule deleted                              -> refused (a change under .sloprail/ re-checks the ADRs)
#   a change outside adr/ and .sloprail/               -> not judged
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=adr-linked
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
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-linked")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING...
  local label="$1" s; shift
  for s in "$@"; do
    jq -es --arg s "$s" 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-linked" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
      { jq -c . "$SR_EVENTS_FILE" >&2; echo "$label: $RULE did not refuse with a reason saying '$s' (sr-checks exit $ran)" >&2; exit 1; }
  done
}
expect_not_judged() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="adr-linked")] | length == 0' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE was asked about a change outside its match" >&2; exit 1; }
}
# adr FOLDER LINKS [SECTIONS] — an ADR.md linking LINKS (a YAML flow list body), with the sections it names
adr() {
  mkdir -p "adr/$1"
  printf -- '---\nconcern: c\nsloprails: [%s]\n---\n## Concern\nc\n%s' "$2" "${3-## Decision$'\n'- d$'\n'}" >"adr/$1/ADR.md"
}

echo base >README
adr good file-guard/file-size
c base
BASE=$(git rev-parse HEAD)

# a well-formed ADR is judged and passed
git checkout -q -b wellformed "$BASE"
adr second file-guard/file-size,file-guard/shapes; c "a well-formed ADR"
run_rule
expect_passed "a well-formed ADR"

# a folder name that is not kebab-case
git checkout -q -b badname "$BASE"
adr Bad_Name file-guard/file-size; c "a badly named ADR"
run_rule
expect_refused "a folder name that is not kebab-case" "adr/Bad_Name: the folder name must be kebab-case starting with a letter"

# no ## Decision section
git checkout -q -b nodecision "$BASE"
adr undecided file-guard/file-size ""; c "an ADR with no decision"
run_rule
expect_refused "an ADR with no Decision section" "adr/undecided has no '## Decision' section"

# links no sloprail
git checkout -q -b nolinks "$BASE"
adr prose ""; c "an ADR nothing enforces"
run_rule
expect_refused "an ADR linking no rule" "adr/prose links no sloprail"

# a link to a rule that does not exist, then the recovery: the link names a rule that does
git checkout -q -b dangling "$BASE"
adr dangling file-guard/nope; c "an ADR linking a missing rule"
run_rule
expect_refused "a link to a missing rule" "adr/dangling links 'file-guard/nope', but .sloprail/file-guard/nope/file-guard.yaml does not exist"
adr dangling file-guard/file-size; c "the link is fixed"
run_rule
expect_passed "the fixed link"

# an unknown nature
git checkout -q -b nature "$BASE"
adr odd rules/file-size; c "an ADR linking an unknown nature"
run_rule
expect_refused "a link of an unknown nature" "adr/odd: 'rules/file-size' must be <file-guard|gate|context>/<name>"

# a linked rule deleted: the ADR that linked it is re-checked and refused
git checkout -q -b deleted "$BASE"
git rm -q -r .sloprail/file-guard/file-size; c "file-size is deleted"
run_rule
expect_refused "a linked rule deleted" "adr/good links 'file-guard/file-size', but .sloprail/file-guard/file-size/file-guard.yaml does not exist"

# the match's edge: a change outside adr/ and .sloprail/ is not judged
git checkout -q -b outside "$BASE"
echo more >>README; c "the readme changes"
run_rule
expect_not_judged "a change outside the match"
