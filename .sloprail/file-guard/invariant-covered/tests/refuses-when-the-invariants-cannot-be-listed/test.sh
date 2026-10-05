#!/usr/bin/env bash
set -euo pipefail

# A rule must refuse when it cannot work out what to check; it never passes on an empty or failed lookup.
# invariant-covered lists the invariants with jq. A jq that fails on that listing used to be an empty loop, and the
# rule passed with every invariant unchecked. The failure is injected: a `jq` shim, first on PATH for the
# sr-checks run only, exits non-zero for the listing programs and otherwise runs the real jq.
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
# shim PROGRAM — a jq that fails when one of its arguments IS PROGRAM (the whole jq program)
shim() {
  mkdir -p "$TMPDIR/shim"
  printf '#!/bin/bash\nfor a in "$@"; do [ "$a" = %q ] && exit 5; done\nexec %q "$@"\n' "$1" "$REAL_JQ" >"$TMPDIR/shim/jq"
  chmod +x "$TMPDIR/shim/jq"
}
run_rule() {   # SHIM_DIR_OR_EMPTY
  : > "$SR_EVENTS_FILE"
  PATH="${1:+$1:}$PATH" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_refused() {   # LABEL SUBSTRING
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="invariant-covered" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: invariant-covered did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}

mkdir -p spec/invariants && echo "base" > README && c base
BASE=$(git rev-parse HEAD)
# an invariant with no implementation and no test: refused when the rule can see it
printf 'statement: it holds\n' > spec/invariants/holds.yaml
c "an invariant, unimplemented and unproven"

# control: nothing injected, the rule refuses for the missing implementation
run_rule ""
expect_refused "control (no failure injected)" "invariant 'holds' has no implementation"

# the id listing fails: refused, saying so, never passed
shim '.[].id'
run_rule "$TMPDIR/shim"
expect_refused "the id listing fails" "the invariants could not be listed, so none could be checked"

# the per-invariant loop's listing fails: refused too
shim '.[]'
run_rule "$TMPDIR/shim"
expect_refused "the loop's listing fails" "the invariants could not be listed, so none could be checked"
