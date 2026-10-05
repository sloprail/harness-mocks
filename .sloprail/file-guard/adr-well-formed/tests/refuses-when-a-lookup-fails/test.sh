#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# adr-well-formed's judge cannot run here (it asks a model), so the case asserts at the level the engine feeds
# it: prepare.sh is run the way the engine runs it (the changeset's payload on stdin, SR_TREE and
# SR_GUARDRAIL_DIR set). The failure is injected: a `jq` shim, first on PATH for that one script run only,
# exits non-zero when an argument holds the marker (a piece of one lookup's jq program) and otherwise runs the
# real jq. A failed listing or count used to read as "no ADR changed": the judge was skipped and the rule passed.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
RULE=adr-well-formed
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
# run_script SCRIPT SHIMMED — runs the rule's script on the changeset's payload; sets out and rc
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

# control: nothing injected, prepare hands the judge the ADR
run_script prepare.sh plain
[ "$rc" -eq 0 ] && jq -e '.additionalContext.subjects | length == 1' <<<"$out" >/dev/null || { echo "control: prepare did not build the judge's input (exit $rc): $out" >&2; exit 1; }

# each lookup of prepare.sh, failed in turn: refused, never "nothing to judge"
shim has 'select(.status != "D"'
run_script prepare.sh shimmed
expect_refused "the changed ADRs' listing fails" "the changed ADRs could not be listed, so none could be judged"
shim is 'length'
run_script prepare.sh shimmed
expect_refused "the count fails" "the changed ADRs could not be counted, so none could be judged"
shim is '.[]'
run_script prepare.sh shimmed
expect_refused "the loop's listing fails" "the changed ADRs could not be listed, so none could be judged"
