#!/usr/bin/env bash
set -euo pipefail

# Fail closed: when a lookup capability-covered makes fails, it refuses (and says what could not be read); it
# never reads a failed lookup as "nothing to check" and passes. The lookups: the capability listing, a
# capability's id, its cell listing and the shape test on its providers (exit 5 is a failure, 1 is a bad
# shape, shapes' finding), each cell read (in the cell loop and in the sr:proves check), a deviation's ADR
# read, and the harness mocks of the tree. The failures are injected with a jq shim, first on PATH for the
# sr-checks run only, that exits non-zero when an argument contains SHIM_JQ_FAIL or is exactly
# SHIM_JQ_FAIL_EXACT, in capability-covered's own scripts only (many rules run in one sr-checks run), after
# letting the first SHIM_JQ_SKIP such calls through. The harness mocks' listing is injected by taking the
# only mock out of the tree.
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REALJQ="$(command -v jq)"
mkdir -p "$TMPDIR/shim"
printf '#!/bin/bash\nhit=""\nfor a in "$@"; do\n  case "$a" in *"${SHIM_JQ_FAIL:-@@none@@}"*) hit=1 ;; esac\n  [ -n "${SHIM_JQ_FAIL_EXACT:-}" ] && [ "$a" = "$SHIM_JQ_FAIL_EXACT" ] && hit=1\ndone\ncase "${SR_GUARDRAIL_DIR:-}" in *capability-covered*) ;; *) hit="" ;; esac\nif [ -n "$hit" ]; then\n  n=$(cat "$SHIM_JQ_COUNT" 2>/dev/null || echo 0); n=$((n + 1)); echo "$n" >"$SHIM_JQ_COUNT"\n  [ "$n" -gt "${SHIM_JQ_SKIP:-0}" ] && exit 5\nfi\nexec "%s" "$@"\n' "$REALJQ" >"$TMPDIR/shim/jq"
chmod +x "$TMPDIR/shim/jq"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
mkdir -p claude-mock/snapshots/runs/r1 claude-mock/internal claude-mock/e2e internal/core spec/capabilities adr/dev-adr
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
printf 'package core\n\n// sr:capability x\nfunc X() {}\n' > internal/core/x.go
printf 'package internal\n\n// sr:provides x/claude\nfunc adapter() {}\n' > claude-mock/internal/x.go
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]\n    deviations:\n      - adr: dev-adr\n        kind: mock-not-modeled\n        statement: the mock leaves something out\n' > spec/capabilities/x.yaml
printf 'package e2e\n\n// sr:proves x/claude\nfunc TestX() {}\n' > claude-mock/e2e/x_test.go
printf -- '---\nconcern: c\n---\n## Decision\n- d\n' > adr/dev-adr/ADR.md
c base
BASE=$(git rev-parse HEAD)

run_rule() {   # ENV=VALUE...
  : > "$SR_EVENTS_FILE"
  env "$@" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_refused() {   # LABEL SUBSTRING
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-covered" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-covered did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
  case "$1" in control*|violation|implemented*|new\ code*) return 0 ;; esac
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .outcome=="refused" and (.reason|contains($s)) and (.reason|contains("could not be evaluated")))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: refused as a verdict, not reported as an error (no verdict to cache)" >&2; exit 1; }
}
expect_not_refused() {   # LABEL SUBSTRING : no refusal says it
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="capability-covered" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null &&
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: capability-covered refused with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
  return 0
}
inject() {   # MARKER [SKIP] — the lookups whose jq program carries MARKER fail, after SKIP of them went through
  rm -f "$TMPDIR/shim.count"
  run_rule "PATH=$TMPDIR/shim:$PATH" "SHIM_JQ_FAIL=$1" "SHIM_JQ_SKIP=${2:-0}" "SHIM_JQ_COUNT=$TMPDIR/shim.count"
}
inject_exact() {   # PROGRAM — the lookups whose jq program is exactly PROGRAM fail
  rm -f "$TMPDIR/shim.count"
  run_rule "PATH=$TMPDIR/shim:$PATH" "SHIM_JQ_FAIL=" "SHIM_JQ_FAIL_EXACT=$1" "SHIM_JQ_SKIP=0" "SHIM_JQ_COUNT=$TMPDIR/shim.count"
}

# the change breaks the cell's coverage: no test proves it any more; with the lookups working that is refused
git checkout -q -b proof-removed "$BASE"
git rm -q claude-mock/e2e/x_test.go; c "x proof removed"

run_rule SHIM_JQ_FAIL=
expect_refused "control: the uninjected run" "no test carries // sr:proves x/claude"

inject_exact '.[]'
expect_refused "a failed listing" "the capability files could not be listed, so nothing could be checked"
inject_exact '.id'
expect_refused "a failed id read" "a capability's id could not be read, so it could not be checked"
inject '.doc.providers | keys[]'
expect_refused "a failed cell listing" "capability 'x': its cells could not be listed, so it could not be checked"
inject '[0].doc.providers'
expect_refused "a failed cell read" "capability 'x' × 'claude': its cell could not be read, so it could not be checked"
inject '.deviations[]?.adr'
expect_refused "a failed deviation read" "capability 'x' × 'claude': its deviations could not be read, so they could not be checked"
# a shape test that fails (exit 5) is a refusal; a providers that is not an object (exit 1) is shapes' finding, not this rule's
inject '(.doc.providers | type) == "object"'
expect_refused "a failed shape test" "capability 'x': its cells could not be read, so it could not be checked"

# the sr:proves check reads the cell too: the proof stays, so the first (cell loop) read goes through and the second fails
git checkout -q -b proof-kept "$BASE"
printf 'package internal\n\n// sr:provides x/claude\n// the adapter, changed\nfunc adapter() {}\n' > claude-mock/internal/x.go; c "the adapter changes"
run_rule SHIM_JQ_FAIL=
expect_not_refused "control: the proof is kept" "could not be"
# (a passed verdict is cached by content, so the injected run is its own commit)
git checkout -q -b proof-kept-2 "$BASE"
printf 'package internal\n\n// sr:provides x/claude\n// the adapter, changed again\nfunc adapter() {}\n' > claude-mock/internal/x.go; c "the adapter changes again"
inject '[0].doc.providers' 1
expect_refused "a failed cell read for a sr:proves marker" "the cell 'x' × 'claude' could not be read, so sr:proves x/claude could not be checked"

# a providers that is not an object is the shapes rule's to report: no "could not be read" from here
git checkout -q -b bad-shape "$BASE"
printf 'statement: x works\nproviders: 3\n' > spec/capabilities/x.yaml; c "x's providers is not an object"
run_rule SHIM_JQ_FAIL=
expect_not_refused "a providers that is not an object" "its cells could not be read"

# no harness mock in the tree: it cannot be worked out what to check
git checkout -q -b no-mocks "$BASE"
git rm -q -r claude-mock; c "the only mock is gone"
run_rule SHIM_JQ_FAIL=
expect_refused "no harness mock" "the harness mocks could not be listed, so it could not be worked out what to check"
