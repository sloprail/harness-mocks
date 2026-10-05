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
reason() { jq -es 'map(select(.kind=="FileGuardChecked" and .rule=="capability-covered" and .outcome=="refused") | .reason) | join("\n")' "$SR_EVENTS_FILE"; }

run_rule SHIM_JQ_FAIL_EXACT=
reason | grep -Fq "no test carries // sr:proves x/claude" ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "control: the uninjected run did not refuse the missing proof" >&2; exit 1; }

run_rule "PATH=$TMPDIR/shim:$PATH" SHIM_JQ_FAIL_EXACT='.[]'
r="$(reason)"
printf '%s' "$r" | grep -Fq "the capability files could not be listed, so nothing could be checked" ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "a failed listing was not refused with its reason (sr-checks exit $ran): $r" >&2; exit 1; }
