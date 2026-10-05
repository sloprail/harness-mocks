#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# adr-conformance's judge cannot run here (it asks a model), so the case asserts at the level the engine feeds
# it: its prepare, _lib/linked-adrs.sh, is run the way the engine runs it (the changeset's payload on stdin,
# SR_TREE and SR_GUARDRAIL_DIR set). The failure is injected: a `jq` shim, first on PATH for that one run only,
# exits non-zero when an argument holds the marker (a piece of one lookup's jq program) and otherwise runs the
# real jq. A failed count used to read as "no ADR links this rule" or "nothing changed": the judge was skipped
# and the rule passed.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
RULE=adr-conformance
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
mkdir -p adr/one internal
printf -- '---\nconcern: c\nsloprails: [file-guard/adr-conformance]\n---\n## Concern\nc\n## Decision\n- d\n' >adr/one/ADR.md
printf 'package internal\n' >internal/x.go
c "an ADR linking the rule, and code it governs"
sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD | jq -c '.payload' >"$TMPDIR/payload.json"

# the script under test is the shared one the rule's prepare names
run_script ../../_lib/linked-adrs.sh plain
[ "$rc" -eq 0 ] && jq -e '.additionalContext.adrs | length == 1' <<<"$out" >/dev/null || { echo "control: linked-adrs.sh did not build the judge's input (exit $rc): $out" >&2; exit 1; }

# each lookup, failed in turn: refused, never "nothing to judge"
shim is 'length'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "the ADR count fails" "the ADRs that link this rule could not be counted, so nothing could be judged"
shim has '.changeset.files | length'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "the file count fails" "the changeset's files could not be counted, so nothing could be judged"
shim is '.changeset.files[]'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "the files' listing fails" "the changeset's files could not be listed, so nothing could be judged"
shim is '.path'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "a file's path cannot be read" "a changed file's path could not be read"
shim is '[.[] | {id, path}]'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "the ADRs cannot be listed for the judge" "the linked ADRs could not be listed for the judge"
shim has 'reduce inputs'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "the loader cannot read the ADR texts" "the ADR texts could not be read, so no ADR could be checked"

shim is '.status'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "a file's status cannot be read" "its status could not be read"
shim is '.oldPath // ""'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "a file's old path cannot be read" "its old path could not be read"
shim is '.diff // ""'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "a file's diff cannot be written" "its diff could not be written for the judge"
shim has '{path: $p, status: $s'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "a file cannot be listed for the judge" "it could not be listed for the judge"
shim has 'additionalContext'
run_script ../../_lib/linked-adrs.sh shimmed
expect_refused "the judge's input cannot be built" "the judge's input could not be built"

# through the engine (`sr-checks run`, as CI runs it): the same failure, injected for that run only, is the rule's
# own refused FileGuardChecked event with the lookup's reason, never a pass. One run per case: a stored verdict is replayed.
shim is \[.\[\]\ \|\ \{id,\ path\}\]
: >"$SR_EVENTS_FILE"
PATH="$TMPDIR/shim:$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es --arg s the\ linked\ ADRs\ could\ not\ be\ listed\ for\ the\ judge 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-conformance" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the engine run: adr-conformance did not refuse with a reason saying 'the linked ADRs could not be listed for the judge' (sr-checks exit $ran)" >&2; exit 1; }
