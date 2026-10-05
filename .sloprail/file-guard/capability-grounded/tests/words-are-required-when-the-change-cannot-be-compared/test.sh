#!/usr/bin/env bash
set -euo pipefail

# Fail closed: added-or-removed.sh is the `when` of the user-words requirement (exit 0 applies it, exit 1 waives
# it). A change it cannot work out (the listing of the changed capability files, or the comparison of the two
# versions fails) applies the requirement, with a hint saying so; it never waives it on an empty loop.
# The script is run as the engine runs it: the payload `sr-checks changeset --rule` gives the subject on stdin,
# SR_TREE and SR_GUARDRAIL_DIR set. The failure is injected with a jq shim, first on PATH for that run only, that
# exits non-zero when an argument contains a marker of the lookup's program and otherwise runs the real jq.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REALJQ="$(command -v jq)"
mkdir -p "$TMPDIR/shim"
printf '#!/bin/bash\nfor a in "$@"; do case "$a" in *"${SHIM_JQ_FAIL:-@@none@@}"*) exit 5 ;; esac; done\nexec "%s" "$@"\n' "$REALJQ" >"$TMPDIR/shim/jq"
chmod +x "$TMPDIR/shim/jq"
git init -q .
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
mkdir -p spec/capabilities claude-mock/snapshots/runs/c claude-mock/snapshots/runs/d
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
printf 'version: 1\n' >claude-mock/snapshots/runs/d/run.yaml
cap() { printf 'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/%s]\n' "$1" >spec/capabilities/c.yaml; }
cap c
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
# a cell cites another recording: the user's words are not required for that
cap d
git add -A && git commit -q -m "c cites another run"

PAYLOAD="$TMPDIR/payload.json"
sr-checks changeset --rule capability-grounded --base "$BASE" --head HEAD | jq -c '.subjects[] | select(.id == "c") | .payload' >"$PAYLOAD"
[ -s "$PAYLOAD" ] || { echo "no subject c" >&2; exit 1; }
when() {   # [ENV=VALUE...] — prints the script's stdout; sets rc
  rc=0
  out="$(env "$@" SR_TREE="$PWD" SR_GUARDRAIL_DIR="$PWD/.sloprail/file-guard/capability-grounded" "$PWD/.sloprail/file-guard/capability-grounded/added-or-removed.sh" <"$PAYLOAD" 2>&1)" || rc=$?
}

# control: nothing here needs words, so the requirement is waived (exit 1)
when SHIM_JQ_FAIL=
[ "$rc" -eq 1 ] || { echo "control: a cell-only change should waive the requirement, exit $rc: $out" >&2; exit 1; }

# the listing of the changed capability files fails: the requirement applies
when "PATH=$TMPDIR/shim:$PATH" 'SHIM_JQ_FAIL=select(test("^spec/capabilities/"))'
{ [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -Fq "the changed capability files could not be listed, so the user's words are required"; } ||
  { echo "a failed listing did not apply the requirement with its hint (exit $rc): $out" >&2; exit 1; }

# the comparison of the two versions fails: the requirement applies
when "PATH=$TMPDIR/shim:$PATH" 'SHIM_JQ_FAIL=$b.statement != $h.statement'
{ [ "$rc" -eq 0 ] && printf '%s' "$out" | grep -Fq "what spec/capabilities/c.yaml changed could not be compared, so the user's words are required"; } ||
  { echo "a failed comparison did not apply the requirement with its hint (exit $rc): $out" >&2; exit 1; }
