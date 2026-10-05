#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# The shared subjects library (_lib/subjects.sh) turns a rule's units into subjects: it resolves each unit's deps (head side)
# paths to object ids and fingerprints them. Every one of its lookups refuses when it fails: a lookup that failed
# and read as empty would mint a subject with no fingerprint (a stale verdict served after a dependency changed) or
# none at all (the judge skipped, the rule passed). invariant-rigor's subjects.sh is run the way the engine runs it (the
# subject's payload on stdin, SR_TREE and SR_GUARDRAIL_DIR set), and the failure is injected by a shim (jq, git,
# awk, sed or mktemp), first on PATH for that one script run only, that fails (or, for `quiet`, prints nothing)
# when an argument holds the marker, a piece of one lookup's program, and otherwise runs the real tool.
# Success is unchanged: the control must print the subject with its fingerprint.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
RULE=invariant-rigor
git init -q -b main .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
c() { git add -A && git commit -q -m "$1"; }
# shim TOOL MARKER [quiet] — TOOL fails (exit 5), or with `quiet` prints nothing and succeeds, when an argument
# contains MARKER (or, when MARKER starts with "=", equals the rest of it) and otherwise runs the real TOOL
shim() {
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

# control: nothing injected, subjects.sh prints the subject, fingerprinted
run_script subjects.sh plain
[ "$rc" -eq 0 ] && jq -e 'length == 1 and .[0].id == "holds" and (.[0].fingerprint | length > 0)' <<<"$out" >/dev/null ||
  { echo "control: subjects.sh did not print the fingerprinted subject (exit $rc): $out" >&2; exit 1; }

# each lookup, failed in turn: refused with its own reason, never a subject without its fingerprint
shim jq '.[].deps'
run_script subjects.sh shimmed
expect_refused "the deps' listing fails" "the subjects' deps could not be listed, so no subject could be made"
shim jq '.[].bdeps'
run_script subjects.sh shimmed
expect_refused "the bdeps' listing fails" "the subjects' bdeps could not be listed, so no subject could be made"
shim jq '.changeset.head'
run_script subjects.sh shimmed
expect_refused "the range's head cannot be read" "the range's head could not be read, so no subject could be made"
shim git '--batch-check'
run_script subjects.sh shimmed
expect_refused "the object ids cannot be read" "could not read object ids at"
shim git '--batch-check' quiet
run_script subjects.sh shimmed
expect_refused "the object ids come back short" "object id lookup at"
shim awk 'missing'
run_script subjects.sh shimmed
expect_refused "the object ids cannot be told apart" "could not read the object ids at"
shim jq '{(.[0]): .[1]}'
run_script subjects.sh shimmed
expect_refused "the deps' ids cannot be paired with their paths" "the deps' object ids could not be read, so no subject could be made"
shim jq '$h[0][.]'
run_script subjects.sh shimmed
expect_refused "the deps cannot be resolved" "the subjects' deps could not be resolved, so no subject could be made"
shim mktemp 'sr-subjects'
run_script subjects.sh shimmed
expect_refused "the fingerprint inputs' directory cannot be made" "cannot make a directory for the subjects' fingerprints"
shim jq 'tojson'
run_script subjects.sh shimmed
expect_refused "the fingerprint inputs cannot be prepared" "the subjects' fingerprints could not be prepared, so no subject could be made"
shim sed 'sr-subjects'
run_script subjects.sh shimmed
expect_refused "the fingerprint inputs cannot be named" "the subjects' fingerprint inputs could not be named"
shim git 'hash-object'
run_script subjects.sh shimmed
expect_refused "the fingerprints cannot be computed" "the subjects' fingerprints could not be computed, so no subject could be made"
shim git 'hash-object' quiet
run_script subjects.sh shimmed
expect_refused "the fingerprints come back short" "the subjects' fingerprints came back the wrong number"
shim jq 'range(0;'
run_script subjects.sh shimmed
expect_refused "the subjects cannot be built" "the subjects could not be built"
shim jq '=length'
run_script subjects.sh shimmed
expect_refused "the subjects cannot be counted" "the subjects could not be counted, so no subject could be made"

# through the engine (`sr-checks run`, as CI runs it): the same failure, injected for that run only, is the rule's own
# refused FileGuardChecked event with the lookup's reason, never a pass. One run per case: a stored verdict is replayed.
shim jq '.[].deps'
: >"$SR_EVENTS_FILE"
PATH="$TMPDIR/shim:$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es --arg s "the subjects' deps could not be listed, so no subject could be made" 'any(.[]; .kind=="FileGuardChecked" and .rule=="invariant-rigor" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the engine run: invariant-rigor did not refuse with a reason saying 'the subjects' deps could not be listed, so no subject could be made' (sr-checks exit $ran)" >&2; exit 1; }
