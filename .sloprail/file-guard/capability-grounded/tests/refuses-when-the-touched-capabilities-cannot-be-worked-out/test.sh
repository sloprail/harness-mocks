#!/usr/bin/env bash
set -euo pipefail

# Fail closed: when the lookup that works out which capabilities a change touches fails, capability-grounded's
# subjects.sh refuses (and says so); it never splits the change into no subjects and judges nothing.
# A judge cannot run here (a model), so this goes at the level the engine reaches first: the `subjects:` script,
# through `sr-checks changeset --rule` (it runs that script and no check, no judge). The failure is injected with
# a jq shim, first on PATH for that call only, that exits non-zero when an argument contains a marker of the
# script's lookup program and otherwise runs the real jq.
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
mkdir -p spec/capabilities claude-mock/snapshots/runs/c
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap "c works"
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
cap "c works, differently"
git add -A && git commit -q -m "c's statement changes"

# control: the subject script names the capability
n="$(sr-checks changeset --rule capability-grounded --base "$BASE" --head HEAD | jq -r '[.subjects[].id] | join(",")')"
[ "$n" = c ] || { echo "control: the subjects were '$n', not c" >&2; exit 1; }

# injected: the lookup program fails
rc=0
out="$(SHIM_JQ_FAIL='harnesses:' PATH="$TMPDIR/shim:$PATH" sr-checks changeset --rule capability-grounded --base "$BASE" --head HEAD 2>&1)" || rc=$?
{ [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -Fq "the capabilities this change touches could not be worked out, so nothing could be judged"; } ||
  { echo "a failed lookup was not refused with its reason (exit $rc): $out" >&2; exit 1; }
