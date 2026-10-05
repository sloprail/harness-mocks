#!/usr/bin/env bash
set -euo pipefail

# Fail closed: when the lookup that lists the capability files fails, capability-covered refuses (and says
# so); it never reads a failed listing as "no capabilities" and passes. The failure is injected with a jq
# shim, first on PATH for the sr-checks run only, that exits non-zero when an argument is exactly `.[]`
# (the listing program) and otherwise runs the real jq.
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REALJQ="$(command -v jq)"
mkdir -p "$TMPDIR/shim"
printf '#!/bin/bash\nfor a in "$@"; do [ -n "${SHIM_JQ_FAIL_EXACT:-}" ] && [ "$a" = "$SHIM_JQ_FAIL_EXACT" ] && exit 5; done\nexec "%s" "$@"\n' "$REALJQ" >"$TMPDIR/shim/jq"
chmod +x "$TMPDIR/shim/jq"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
mkdir -p claude-mock/snapshots/runs/r1 claude-mock/internal claude-mock/e2e internal/core spec/capabilities
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
printf 'package core\n\n// sr:capability x\nfunc X() {}\n' > internal/core/x.go
printf 'package internal\n\n// sr:provides x/claude\nfunc adapter() {}\n' > claude-mock/internal/x.go
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]\n' > spec/capabilities/x.yaml
printf 'package e2e\n\n// sr:proves x/claude\nfunc TestX() {}\n' > claude-mock/e2e/x_test.go
c base
BASE=$(git rev-parse HEAD)
# the change breaks the cell's coverage: no test proves it any more; with the listing working that is refused
git rm -q claude-mock/e2e/x_test.go; c "x proof removed"

run_rule() {   # ENV=VALUE...
  : > "$SR_EVENTS_FILE"
  env "$@" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_refused() {   # LABEL SUBSTRING
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-covered" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-covered did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}

run_rule SHIM_JQ_FAIL_EXACT=
expect_refused "control: the uninjected run" "no test carries // sr:proves x/claude"

run_rule "PATH=$TMPDIR/shim:$PATH" SHIM_JQ_FAIL_EXACT='.[]'
expect_refused "a failed listing" "the capability files could not be listed, so nothing could be checked"
