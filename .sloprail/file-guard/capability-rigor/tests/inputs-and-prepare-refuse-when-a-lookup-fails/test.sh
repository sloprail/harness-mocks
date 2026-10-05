#!/usr/bin/env bash
set -euo pipefail

# Fail closed: capability-rigor's inputs-ready.sh (a script check, ahead of the judge) and prepare.sh (which feeds
# the judge) refuse when a lookup they depend on fails, naming what could not be read; they never read it as an
# empty list (nothing touched, so no input to check, or the model skipped) and pass. A judge cannot run here
# (a model), so both scripts are run as the engine runs them: the payload `sr-checks changeset --rule` gives the
# subject on stdin, SR_TREE and SR_GUARDRAIL_DIR set. The failures are injected with a jq shim, first on PATH for
# that run only, that exits non-zero when an argument contains a marker of one lookup's program and otherwise runs
# the real jq.
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
mkdir -p spec/capabilities claude-mock/snapshots/runs/c/samples/20240101-000000 claude-mock/e2e
printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$SHA" >claude-mock/snapshots/MANIFEST.yaml
printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
printf '{"e":1}\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/events.jsonl
printf 'sealed\n' >claude-mock/snapshots/runs/c/samples/20240101-000000/SEAL
printf 'package e2e\n\n// sr:proves c/claude\nfunc TestC() {}\n' >claude-mock/e2e/c_test.go
cap() { printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml; }
cap "c works"
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
cap "c works, differently"
git add -A && git commit -q -m "c's statement changes"

PAYLOAD="$TMPDIR/payload.json"
sr-checks changeset --rule capability-rigor --base "$BASE" --head HEAD | jq -c '.subjects[] | select(.id == "c") | .payload' >"$PAYLOAD"
[ -s "$PAYLOAD" ] || { echo "no subject c" >&2; exit 1; }
script() {   # SCRIPT [ENV=VALUE...] — prints its stdout and stderr; sets rc
  local s="$1"; shift
  rc=0
  out="$(env "$@" SR_TREE="$PWD" SR_GUARDRAIL_DIR="$PWD/.sloprail/file-guard/capability-rigor" "$PWD/.sloprail/file-guard/capability-rigor/$s" <"$PAYLOAD" 2>&1)" || rc=$?
}
expect_refused() {   # SCRIPT MARKER REASON — with the lookup carrying MARKER failing, SCRIPT refuses saying REASON
  script "$1" "PATH=$TMPDIR/shim:$PATH" "SHIM_JQ_FAIL=$2"
  { [ "$rc" -ne 0 ] && printf '%s' "$out" | grep -Fq "$3"; } ||
    { echo "$1: a failed lookup ($2) was not refused saying '$3' (exit $rc): $out" >&2; exit 1; }
}

# control: every input is there, and the subject is prepared with its statement and its doc
script inputs-ready.sh SHIM_JQ_FAIL=
[ "$rc" -eq 0 ] || { echo "control: inputs-ready refused (exit $rc): $out" >&2; exit 1; }
script prepare.sh SHIM_JQ_FAIL=
{ [ "$rc" -eq 0 ] && printf '%s' "$out" | jq -e '.additionalContext.subjects[0] | .id == "c/claude" and .context.statement == "c works, differently" and (.context.docs | length) == 1' >/dev/null; } ||
  { echo "control: the subject was not prepared (exit $rc): $out" >&2; exit 1; }

# the pairs lookup fails
expect_refused inputs-ready.sh '$want == ""' "the touched capability pairs could not be worked out, so the judge's inputs could not be checked"
expect_refused prepare.sh '$want == ""' "the touched capability pairs could not be worked out, so nothing could be prepared for the judge"
# a cell's cited docs and runs cannot be listed
expect_refused inputs-ready.sh "(.docs // [])" "c/claude: its cited docs could not be listed, so they could not be checked"
expect_refused inputs-ready.sh "(.runs // [])" "c/claude: its cited runs could not be listed, so they could not be checked"
expect_refused prepare.sh "(.docs // [])" "c/claude: its cited docs could not be listed, so it could not be prepared for the judge"
expect_refused prepare.sh "(.runs // [])" "c/claude: its cited runs could not be listed, so it could not be prepared for the judge"
# the statement, and the assembling of the context
expect_refused prepare.sh ".doc.statement" "c/claude: its statement could not be read, so it could not be prepared for the judge"
expect_refused prepare.sh 'context: {harness' "c/claude: its context could not be assembled, so it could not be prepared for the judge"
