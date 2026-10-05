#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# adr-linked checks every ADR in the tree. A jq that fails on listing them used to be an empty loop, and the rule
# passed with every ADR unchecked. links-resolve.sh is run the way the engine runs it (the changeset's payload on
# stdin, SR_TREE and SR_GUARDRAIL_DIR set); the failure is injected: a `jq` shim, first on PATH for that one run
# only, exits non-zero when an argument holds the marker (a piece of one lookup's jq program) and otherwise runs
# the real jq. The shared ADR loader (_lib/adr.sh) is covered the same way.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
RULE=adr-linked
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
mkdir -p adr/one
printf -- '---\nconcern: c\nsloprails: [file-guard/adr-linked]\n---\n## Concern\nc\n## Decision\n- d\n' >adr/one/ADR.md
c "an ADR"
sr-checks changeset --rule "$RULE" --base "$BASE" --head HEAD | jq -c '.payload' >"$TMPDIR/payload.json"

# control: nothing injected, a well-formed ADR passes
run_script links-resolve.sh plain
[ "$rc" -eq 0 ] || { echo "control: links-resolve.sh refused a well-formed ADR (exit $rc): $out" >&2; exit 1; }

# each lookup, failed in turn: refused, never an empty list of ADRs
shim is '.[]'
run_script links-resolve.sh shimmed
expect_refused "the ADRs' listing fails" "the ADRs could not be listed, so none could be checked"
shim is '.id'
run_script links-resolve.sh shimmed
expect_refused "an ADR's id cannot be read" "an ADR's id could not be read, so it could not be checked"
shim is '.frontmatter'
run_script links-resolve.sh shimmed
expect_refused "an ADR's frontmatter cannot be read" "its frontmatter could not be read, so it could not be checked"
shim is '.text'
run_script links-resolve.sh shimmed
expect_refused "an ADR's text cannot be read" "its text could not be read, so it could not be checked"
shim has 'reduce inputs'
run_script links-resolve.sh shimmed
expect_refused "the loader cannot read the ADR texts" "the ADR texts could not be read, so no ADR could be checked"
shim has 'ltrimstr($root)'
run_script links-resolve.sh shimmed
expect_refused "the loader cannot list the ADRs" "the ADRs could not be listed, so no ADR could be checked"

shim has 'if (.sloprails | type)'
run_script links-resolve.sh shimmed
expect_refused "an ADR's linked rules cannot be read" "its linked rules could not be read, so it could not be checked"

# through the engine (`sr-checks run`, as CI runs it): the same failure, injected for that run only, is the rule's
# own refused FileGuardChecked event with the lookup's reason, never a pass. One run per case: a stored verdict is replayed.
shim is .\[\]
: >"$SR_EVENTS_FILE"
PATH="$TMPDIR/shim:$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es --arg s the\ ADRs\ could\ not\ be\ listed,\ so\ none\ could\ be\ checked 'any(.[]; .kind=="FileGuardChecked" and .rule=="adr-linked" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the engine run: adr-linked did not refuse with a reason saying 'the ADRs could not be listed, so none could be checked' (sr-checks exit $ran)" >&2; exit 1; }
