#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# adr-matches-sloprails' judge cannot run here (it asks a model), so the case asserts at the level the engine
# feeds it: `sr-checks changeset --rule` runs the rule's subjects.sh, and prepare.sh is run the way the engine
# runs it (the subject's payload on stdin, SR_TREE and SR_GUARDRAIL_DIR set). The failure is injected: a `jq`
# shim, first on PATH for that one script run only, exits non-zero when an argument holds the marker (a piece of
# one lookup's jq program) and otherwise runs the real jq. A failed listing used to read as "no ADR to judge": the
# judge was skipped and the rule passed. The same goes for the shared ADR loader (_lib/adr.sh).
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
RULE=adr-matches-sloprails
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
mkdir -p adr/one adr/two
printf -- '---\nconcern: c\nsloprails: [file-guard/invariant-covered]\n---\n## Concern\nc\n## Decision\n- d\n' >adr/one/ADR.md
printf -- '---\nconcern: c\nsloprails: [file-guard/invariant-covered]\n---\n## Concern\nc\n## Decision\n- d\n' >adr/two/ADR.md
c "two ADRs enforced by the same rule"
sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD | jq -c '.subjects[] | select(.id == "one") | .payload' >"$TMPDIR/payload.json"
[ "$(jq -r .subject.id "$TMPDIR/payload.json")" = one ] || { echo "no subject 'one'" >&2; exit 1; }

# control: nothing injected, prepare hands the judge its input
run_script prepare.sh plain
[ "$rc" -eq 0 ] && jq -e '.additionalContext.subjects | length == 1' <<<"$out" >/dev/null || { echo "control: prepare did not build the judge's input (exit $rc): $out" >&2; exit 1; }

# each lookup of prepare.sh (and of the ADR loader it sources), failed in turn: refused, never "nothing to judge"
shim is '.[]'
run_script prepare.sh shimmed
expect_refused "the ADRs' listing fails" "the ADRs could not be listed, so none could be judged"
shim is '.id'
run_script prepare.sh shimmed
expect_refused "an ADR's id cannot be read" "an ADR's id could not be read, so it could not be judged"
shim is '.frontmatter.sloprails // [] | .[]'
run_script prepare.sh shimmed
expect_refused "an ADR's links cannot be read" "its linked rules could not be read, so it could not be judged"
shim has '[.[] | select(.id != $id'
run_script prepare.sh shimmed
expect_refused "the other ADRs of a rule cannot be listed" "the other ADRs linking file-guard/invariant-covered could not be listed, so it could not be judged"
shim has 'reduce inputs'
run_script prepare.sh shimmed
expect_refused "the loader cannot read the ADR texts" "the ADR texts could not be read, so no ADR could be checked"
shim has 'ltrimstr($root)'
run_script prepare.sh shimmed
expect_refused "the loader cannot list the ADRs" "the ADRs could not be listed, so no ADR could be checked"

# subjects.sh, through the engine: a failed listing refuses the rule, never "no subjects"
shim has '$adrs0[0]'
out="$(PATH="$TMPDIR/shim:$PATH" sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD 2>&1)" && rc=0 || rc=$?
expect_refused "subjects.sh's listing fails" "the touched ADRs could not be worked out, so no subject could be made"

# through the engine (`sr-checks run`, as CI runs it): the same failure, injected for that run only, is the rule's
# own refused FileGuardChecked event with the lookup's reason, never a pass. One run per case: a stored verdict is replayed.
shim has \$adrs0\[0\]
: >"$SR_EVENTS_FILE"
PATH="$TMPDIR/shim:$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es --arg s the\ touched\ ADRs\ could\ not\ be\ worked\ out,\ so\ no\ subject\ could\ be\ made 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-matches-sloprails" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the engine run: adr-matches-sloprails did not refuse with a reason saying 'the touched ADRs could not be worked out, so no subject could be made' (sr-checks exit $ran)" >&2; exit 1; }
