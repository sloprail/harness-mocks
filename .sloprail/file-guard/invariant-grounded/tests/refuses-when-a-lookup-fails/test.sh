#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# invariant-grounded's judge cannot run here (it asks a model), so the case asserts at the level the engine feeds
# it: `sr-checks changeset --rule` runs the rule's subjects.sh, and prepare.sh is run the way the engine runs it
# (the subject's payload on stdin, SR_TREE and SR_GUARDRAIL_DIR set). The failure is injected: a `jq` shim, first
# on PATH for that one script run only, exits non-zero when an argument holds the marker (a piece of one lookup's
# jq program) and otherwise runs the real jq. A failed listing or count used to read as "no invariant changed":
# the judge was skipped and the rule passed.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
RULE=invariant-grounded
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
mkdir -p spec/invariants
printf 'statement: it holds\n' >spec/invariants/holds.yaml
c "an invariant"
sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD | jq -c '.subjects[0].payload' >"$TMPDIR/payload.json"
[ "$(jq -r .subject.id "$TMPDIR/payload.json")" = holds ] || { echo "the subject is not 'holds'" >&2; exit 1; }

# control: nothing injected, prepare hands the judge the invariant
run_script prepare.sh plain
[ "$rc" -eq 0 ] && jq -e '.additionalContext.subjects | length == 1' <<<"$out" >/dev/null || { echo "control: prepare did not build the judge's input (exit $rc): $out" >&2; exit 1; }

# each lookup of prepare.sh, failed in turn: refused, never "nothing to judge"
shim has 'select(.path | test("^spec/invariants/'
run_script prepare.sh shimmed
expect_refused "the changed files' listing fails" "the changed invariant files could not be listed, so none could be judged"
shim is 'length'
run_script prepare.sh shimmed
expect_refused "the count fails" "the changed invariant files could not be counted, so none could be judged"
shim is '.[]'
run_script prepare.sh shimmed
expect_refused "the loop's listing fails" "the changed invariant files could not be listed, so none could be judged"

# subjects.sh, through the engine: a failed listing refuses the rule, never "no subjects"
shim has 'bdeps: [.path]'
out="$(PATH="$TMPDIR/shim:$PATH" sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD 2>&1)" && rc=0 || rc=$?
expect_refused "subjects.sh's listing fails" "the changed invariant files could not be listed, so no subject could be made"
