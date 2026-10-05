#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# adr-grounded's judge cannot run here (it asks a model), so the case asserts at the level the engine feeds
# it: `sr-checks changeset --rule` runs the rule's subjects.sh, and prepare.sh is run the way the engine runs it
# (the subject's payload on stdin, SR_TREE and SR_GUARDRAIL_DIR set). The failure is injected: a `jq` shim, first
# on PATH for that one script run only, exits non-zero when an argument holds the marker (a piece of one lookup's
# jq program) and otherwise runs the real jq. A failed listing or count used to read as "no ADR changed": the
# judge was skipped and the rule passed.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
RULE=adr-grounded
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
c() { git add -A && git commit -q -m "$1"; }
# shim MODE MARKER — a jq that fails when an argument contains (has) or equals (is) MARKER
shim() {
  mkdir -p "$TMPDIR/shim"
  printf '%s' "$2" >"$TMPDIR/shim/marker"; printf '%s' "$1" >"$TMPDIR/shim/mode"
  cat >"$TMPDIR/shim/jq" <<EOS
#!/bin/bash
m="\$(cat "$TMPDIR/shim/marker")"; mode="\$(cat "$TMPDIR/shim/mode")"
for a in "\$@"; do
  if [ "\$mode" = is ]; then [ "\$a" = "\$m" ] && exit 5; else case "\$a" in *"\$m"*) exit 5 ;; esac; fi
done
exec $REAL_JQ "\$@"
EOS
  chmod +x "$TMPDIR/shim/jq"
}
# run_script SCRIPT SHIMMED — runs the rule's script on the head subject's payload; sets out and rc
run_script() {
  local p="$PATH"; [ "$2" = shimmed ] && p="$TMPDIR/shim:$PATH"
  out="$(cd ".sloprail/file-guard/$RULE" && SR_TREE="$ROOT" SR_GUARDRAIL_DIR="$PWD" PATH="$p" bash "./$1" <"$TMPDIR/payload.json" 2>&1)" && rc=0 || rc=$?
}
expect_refused() {   # LABEL SUBSTRING
  [ "$rc" -ne 0 ] && [[ "$out" == *"$2"* ]] || { echo "$1: wanted a refusal saying '$2', got exit $rc: $out" >&2; exit 1; }
}

ROOT="$PWD"
mkdir -p adr/one
printf -- '---\nconcern: c\nsloprails: [file-guard/adr-linked]\nexceptions:\n  - internal/old.go\n---\n## Concern\nc\n## Decision\n- d\n' >adr/one/ADR.md
c base
BASE=$(git rev-parse HEAD)
printf -- '---\nconcern: c\nsloprails: [file-guard/adr-linked]\n---\n## Concern\nc\n## Decision\n- d\n' >adr/one/ADR.md
c "the legacy exception goes"
sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD | jq -c '.subjects[0].payload' >"$TMPDIR/payload.json"
[ "$(jq -r .subject.id "$TMPDIR/payload.json")" = one ] || { echo "the subject is not 'one'" >&2; exit 1; }

# control: nothing injected, prepare hands the judge the ADR
run_script prepare.sh plain
[ "$rc" -eq 0 ] && jq -e '.additionalContext.subjects | length == 1' <<<"$out" >/dev/null || { echo "control: prepare did not build the judge's input (exit $rc): $out" >&2; exit 1; }

# each lookup of prepare.sh, failed in turn: refused, never "nothing to judge"
shim has '[.[] | select($want == ""'
run_script prepare.sh shimmed
expect_refused "the selection fails" "the changed ADR files could not be selected, so none could be judged"
shim is 'length'
run_script prepare.sh shimmed
expect_refused "the count fails" "the changed ADR files could not be counted, so none could be judged"
shim is '.[]'
run_script prepare.sh shimmed
expect_refused "the loop's listing fails" "the changed ADR files could not be listed, so none could be judged"
shim is '{adr, path, status}'
run_script prepare.sh shimmed
expect_refused "a file's fields cannot be read" "its fields could not be read"

shim is '.path'
run_script prepare.sh shimmed
expect_refused "a file's path cannot be read" "a changed ADR file's path could not be read"
shim is '.diff // ""'
run_script prepare.sh shimmed
expect_refused "a file's diff cannot be written" "its diff could not be written for the judge"
shim is '.oldContent // ""'
run_script prepare.sh shimmed
expect_refused "a file's old content cannot be written" "its old content could not be written for the judge"
shim has '. + [{adr: $m.adr'
run_script prepare.sh shimmed
expect_refused "a file cannot be listed for the judge" "it could not be listed for the judge"
shim has 'group_by(.adr)'
run_script prepare.sh shimmed
expect_refused "the ADR subjects cannot be built" "the ADR subjects could not be built"
shim has 'additionalContext'
run_script prepare.sh shimmed
expect_refused "the judge's input cannot be built" "the judge's input could not be built"

# subjects.sh, through the engine: a failed listing refuses the rule, never "no subjects"
shim has 'group_by(.id)'
out="$(PATH="$TMPDIR/shim:$PATH" sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD 2>&1)" && rc=0 || rc=$?
expect_refused "subjects.sh's listing fails" "the touched ADRs could not be worked out, so no subject could be made"

# through the engine (`sr-checks run`, as CI runs it): the same failure, injected for that run only, is the rule's
# own refused FileGuardChecked event with the lookup's reason, never a pass. One run per case: a stored verdict is replayed.
shim has group_by\(.id\)
: >"$SR_EVENTS_FILE"
PATH="$TMPDIR/shim:$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es --arg s the\ touched\ ADRs\ could\ not\ be\ worked\ out,\ so\ no\ subject\ could\ be\ made 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-grounded" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the engine run: adr-grounded did not refuse with a reason saying 'the touched ADRs could not be worked out, so no subject could be made' (sr-checks exit $ran)" >&2; exit 1; }
