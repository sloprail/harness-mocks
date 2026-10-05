#!/usr/bin/env bash
set -euo pipefail

# Fail closed: capability-grounded's prepare.sh feeds the judge. A lookup it cannot make (the capability list, a
# cell's docs or runs, the statement, the assembled context) refuses, naming what could not be read; it never
# hands the judge less (an empty statement, no docs, no capabilities) and lets the verdict pass on that.
# A judge cannot run here (a model), so the prepare step is run as the engine runs it: the payload
# `sr-checks changeset --rule` gives the subject on stdin, SR_TREE and SR_GUARDRAIL_DIR set. The failures are
# injected with a jq shim, first on PATH for that run only, that exits non-zero when an argument contains a
# marker of one lookup's program and otherwise runs the real jq.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REALJQ="$(command -v jq)"
mkdir -p "$TMPDIR/shim"
printf '#!/bin/bash\nfor a in "$@"; do case "$a" in *"${SHIM_JQ_FAIL:-@@none@@}"*) exit 5 ;; esac; done\nexec "%s" "$@"\n' "$REALJQ" >"$TMPDIR/shim/jq"
chmod +x "$TMPDIR/shim/jq"
# a page of the vendor's docs: prepare.sh reads the frozen text, fetched on a miss
mkdir -p bin
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nfrozen text\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
export PATH="$PWD/bin:$PATH"
SHA="$(printf '# s\nfrozen text\n' | shasum -a 256 | cut -d' ' -f1)"
git init -q .
printf 'bin\n' >.gitignore
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
mkdir -p spec/capabilities claude-mock/snapshots/runs/c
printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$SHA" >claude-mock/snapshots/MANIFEST.yaml
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap "c works"
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
cap "c works, differently"
git add -A && git commit -q -m "c's statement changes"

PAYLOAD="$TMPDIR/payload.json"
sr-checks changeset --rule capability-grounded --base "$BASE" --head HEAD | jq -c '.subjects[] | select(.id == "c") | .payload' >"$PAYLOAD"
[ -s "$PAYLOAD" ] || { echo "no subject c" >&2; exit 1; }
prepare() {   # [ENV=VALUE...] — prints the script's stdout and stderr; sets rc
  rc=0
  out="$(env "$@" SR_TREE="$PWD" SR_GUARDRAIL_DIR="$PWD/.sloprail/file-guard/capability-grounded" "$PWD/.sloprail/file-guard/capability-grounded/prepare.sh" <"$PAYLOAD" 2>&1)" || rc=$?
}

# control: the subject is prepared, with its statement and its doc
prepare SHIM_JQ_FAIL=
{ [ "$rc" -eq 0 ] && printf '%s' "$out" | jq -e '.additionalContext.subjects[0] | .id == "c" and .statement == "c works, differently" and (.docs | length) == 1' >/dev/null; } ||
  { echo "control: the subject was not prepared (exit $rc): $out" >&2; exit 1; }

expect_refused() {   # MARKER REASON — with the lookup carrying MARKER failing, prepare refuses saying REASON
  prepare "PATH=$TMPDIR/shim:$PATH" "SHIM_JQ_FAIL=$1"
  { [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -Fq "$2"; } ||
    { echo "a failed lookup ($1) was not refused saying '$2' (exit $rc): $out" >&2; exit 1; }
}
expect_refused '.[]' "the capability files could not be listed, so nothing could be prepared for the judge"
expect_refused "(.value.docs // [])" "c: its cited docs could not be listed, so it could not be prepared for the judge"
expect_refused '.doc.statement' "c: its statement could not be read, so it could not be prepared for the judge"
expect_refused 'select(.value == "pending")' "c: its pending cells could not be read, so it could not be prepared for the judge"
expect_refused 'removed: false' "c: its context could not be assembled, so it could not be prepared for the judge"
