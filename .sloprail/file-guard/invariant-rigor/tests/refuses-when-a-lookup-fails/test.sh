#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# invariant-rigor's judge cannot run here (it asks a model), so the case asserts at the level the engine feeds
# it: `sr-checks changeset --rule` runs the rule's subjects.sh, and prepare.sh is run the way the engine runs it
# (the subject's payload on stdin, SR_TREE and SR_GUARDRAIL_DIR set). The failure is injected: a `jq` shim, first
# on PATH for that one script run only, exits non-zero when an argument holds the marker (a piece of one lookup's
# jq program) and otherwise runs the real jq. A failed lookup used to read as "nothing touched": the judge was
# skipped and the rule passed.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
RULE=invariant-rigor
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
echo base >README; c base
BASE=$(git rev-parse HEAD)
mkdir -p spec/invariants internal
printf 'statement: it holds\n' >spec/invariants/holds.yaml
printf 'package internal\n\n// sr:invariant holds\nfunc holds() {}\n' >internal/holds.go
printf 'package internal\n\n// sr:proves holds\nfunc TestHolds() {}\n' >internal/holds_test.go
c "an invariant, implemented and proven"
sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD | jq -c '.subjects[0].payload' >"$TMPDIR/payload.json"
[ "$(jq -r .subject.id "$TMPDIR/payload.json")" = holds ] || { echo "the subject is not 'holds'" >&2; exit 1; }

# control: nothing injected, prepare hands the judge the invariant
run_script prepare.sh plain
[ "$rc" -eq 0 ] && jq -e '.additionalContext.subjects | length == 1' <<<"$out" >/dev/null || { echo "control: prepare did not build the judge's input (exit $rc): $out" >&2; exit 1; }

# each lookup of prepare.sh, failed in turn: refused, never "nothing to judge"
shim has 'ltrimstr("spec/invariants/")'
run_script prepare.sh shimmed
expect_refused "the changed invariant files' listing fails" "the changed invariant files could not be listed, so no invariant could be judged"
shim has '.fqn | select(test("/") | not)'
run_script prepare.sh shimmed
expect_refused "the markers' listing fails" "the changed sr:invariant / sr:proves markers could not be listed, so no invariant could be judged"
shim has 'doc.statement'
run_script prepare.sh shimmed
expect_refused "the statement's lookup fails" "invariant 'holds': its statement could not be read, so it could not be judged"
shim has 'map(select(. != ""))'
run_script prepare.sh shimmed
expect_refused "the proving tests' listing fails" "invariant 'holds': its proving tests could not be listed, so it could not be judged"

# subjects.sh, through the engine: a failed listing refuses the rule, never "no subjects"
shim has '$pv['
out="$(PATH="$TMPDIR/shim:$PATH" sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD 2>&1)" && rc=0 || rc=$?
expect_refused "subjects.sh's listing fails" "the touched invariants could not be worked out, so no subject could be made"

# through the engine (`sr-checks run`, as CI runs it): the same failure, injected for that run only, is the rule's
# own refused FileGuardChecked event with the lookup's reason, never a pass. One run per case: a stored verdict is replayed.
shim has \$pv\[
: >"$SR_EVENTS_FILE"
PATH="$TMPDIR/shim:$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es --arg s the\ touched\ invariants\ could\ not\ be\ worked\ out,\ so\ no\ subject\ could\ be\ made 'any(.[]; .kind=="FileGuardChecked" and .rule=="invariant-rigor" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the engine run: invariant-rigor did not refuse with a reason saying 'the touched invariants could not be worked out, so no subject could be made' (sr-checks exit $ran)" >&2; exit 1; }
