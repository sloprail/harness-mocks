#!/usr/bin/env bash
set -euo pipefail

# Fail closed: when the lookup that works out the (capability, harness) pairs a change touches fails,
# capability-rigor's subjects.sh refuses (and says so); it never reads the failure as "no pairs", which is the
# `unclaimed` subject: a split that judges nothing. A judge cannot run here (a model), so this goes at the level
# the engine reaches first: the `subjects:` script, through `sr-checks changeset --rule` (it runs that script and
# no check, no judge). The failures are injected with a jq shim, first on PATH for that call only, that exits
# non-zero when an argument contains a marker of one lookup's program and otherwise runs the real jq.
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
mkdir -p spec/capabilities claude-mock/snapshots/runs/c/samples/20240101-000000 claude-mock/e2e
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
printf '{"e":1}\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
printf 'package e2e\n\n// sr:proves c/claude\nfunc TestC() {}\n' >claude-mock/e2e/c_test.go
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap "c works"
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
cap "c works, differently"
git add -A && git commit -q -m "c's statement changes"

# control: the subject script names the capability
n="$(sr-checks changeset --rule capability-rigor --base "$BASE" --head HEAD | jq -r '[.subjects[].id] | join(",")')"
[ "$n" = c ] || { echo "control: the subjects were '$n', not c" >&2; exit 1; }

expect_refused() {   # MARKER REASON — with the lookup carrying MARKER failing, the script refuses saying REASON
  local rc=0 out
  out="$(SHIM_JQ_FAIL="$1" PATH="$TMPDIR/shim:$PATH" sr-checks changeset --rule capability-rigor --base "$BASE" --head HEAD 2>&1)" || rc=$?
  { [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -Fq "$2"; } ||
    { echo "a failed lookup ($1) was not refused saying '$2' (exit $rc): $out" >&2; exit 1; }
}
expect_refused '$want == ""' "the touched capability pairs could not be worked out, so nothing could be judged"
expect_refused '"pairs:"' "the capabilities this change touches could not be worked out, so nothing could be judged"
