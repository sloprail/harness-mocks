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
  out="$(cd ".sloprail/file-guard/$RULE" && SR_TREE="$ROOT" SR_GUARDRAIL_DIR="$PWD" PATH="$p" bash "./$1" <"${3:-$TMPDIR/payload.json}" 2>&1)" && rc=0 || rc=$?
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

shim is 'length'
run_script prepare.sh shimmed
expect_refused "the invariants to judge cannot be counted" "the invariants to judge could not be counted"
shim has '. + [{id: $id, path: $p'
run_script prepare.sh shimmed
expect_refused "an invariant cannot be listed for the judge" "it could not be listed for the judge"
shim has 'additionalContext'
run_script prepare.sh shimmed
expect_refused "the judge's input cannot be built" "the judge's input could not be built"
shim is '-sc'
run_script prepare.sh shimmed
expect_refused "the specs cannot be read" "the specs under spec/invariants could not be read, so none could be checked"

# the shared subjects library (_lib/subjects.sh), through subjects.sh run directly on the head subject's payload: each of its lookups
# refuses with its own reason when it fails, never a subject without its fingerprint (a stale verdict served after a dependency
# changed) or none at all. The shims here are jq, git, awk, sed and mktemp (a git shim would break the engine, so no `sr-checks`
# runs under it); one that `quiet`ly prints nothing and succeeds makes a lookup come back short.
# shimtool TOOL MARKER [quiet] — TOOL fails (exit 5), or with `quiet` prints nothing and succeeds, when an argument
# contains MARKER (or, when MARKER starts with "=", equals the rest of it) and otherwise runs the real TOOL
shimtool() {
  local act=5; [ -z "${3:-}" ] || act=0
  rm -rf "$TMPDIR/shim"; mkdir -p "$TMPDIR/shim"
  printf '%s' "$2" >"$TMPDIR/shim/marker"
  cat >"$TMPDIR/shim/$1" <<EOS
#!/bin/bash
m="\$(cat "$TMPDIR/shim/marker")"
for a in "\$@"; do
  if [ "\${m:0:1}" = "=" ]; then [ "\$a" = "\${m:1}" ] && exit $act
  else case "\$a" in *"\$m"*) exit $act ;; esac; fi
done
exec $(command -v "$1") "\$@"
EOS
  chmod +x "$TMPDIR/shim/$1"
}
# control: nothing injected, subjects.sh prints the subject, fingerprinted
run_script subjects.sh plain
[ "$rc" -eq 0 ] && jq -e 'length == 1 and .[0].id == "holds" and (.[0].fingerprint | length > 0)' <<<"$out" >/dev/null ||
  { echo "control: subjects.sh did not print the fingerprinted subject (exit $rc): $out" >&2; exit 1; }

# each lookup, failed in turn: refused with its own reason, never a subject without its fingerprint
shimtool jq '.[].deps'
run_script subjects.sh shimmed
expect_refused "the deps' listing fails" "the subjects' deps could not be listed, so no subject could be made"
shimtool jq '.[].bdeps'
run_script subjects.sh shimmed
expect_refused "the bdeps' listing fails" "the subjects' bdeps could not be listed, so no subject could be made"
shimtool jq '.changeset.head'
run_script subjects.sh shimmed
expect_refused "the range's head cannot be read" "the range's head could not be read, so no subject could be made"
shimtool git '--batch-check'
run_script subjects.sh shimmed
expect_refused "the object ids cannot be read" "could not read object ids at"
shimtool git '--batch-check' quiet
run_script subjects.sh shimmed
expect_refused "the object ids come back short" "object id lookup at"
shimtool awk 'missing'
run_script subjects.sh shimmed
expect_refused "the object ids cannot be told apart" "could not read the object ids at"
shimtool jq '{(.[0]): .[1]}'
run_script subjects.sh shimmed
expect_refused "the deps' ids cannot be paired with their paths" "the deps' object ids could not be read, so no subject could be made"
shimtool jq '$h[0][.]'
run_script subjects.sh shimmed
expect_refused "the deps cannot be resolved" "the subjects' deps could not be resolved, so no subject could be made"
shimtool mktemp 'sr-subjects'
run_script subjects.sh shimmed
expect_refused "the fingerprint inputs' directory cannot be made" "cannot make a directory for the subjects' fingerprints"
shimtool jq 'tojson'
run_script subjects.sh shimmed
expect_refused "the fingerprint inputs cannot be prepared" "the subjects' fingerprints could not be prepared, so no subject could be made"
shimtool sed 'sr-subjects'
run_script subjects.sh shimmed
expect_refused "the fingerprint inputs cannot be named" "the subjects' fingerprint inputs could not be named"
shimtool git 'hash-object'
run_script subjects.sh shimmed
expect_refused "the fingerprints cannot be computed" "the subjects' fingerprints could not be computed, so no subject could be made"
shimtool git 'hash-object' quiet
run_script subjects.sh shimmed
expect_refused "the fingerprints come back short" "the subjects' fingerprints came back the wrong number"
shimtool jq 'range(0;'
run_script subjects.sh shimmed
expect_refused "the subjects cannot be built" "the subjects could not be built"
shimtool jq '=length'
run_script subjects.sh shimmed
expect_refused "the subjects cannot be counted" "the subjects could not be counted, so no subject could be made"

rm -rf "$TMPDIR/shim"

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
