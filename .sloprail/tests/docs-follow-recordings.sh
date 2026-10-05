#!/usr/bin/env bash
# Rule and script tests for "docs follow recordings" (adr/pinned-harness-versions): recordings own
# the truth, so an upstream doc change is no PR's problem.
#   1. capture.sh re-freezes the docs its harness's cells cite, together with a recording (and
#      leaves an already-frozen page, its fetch date included, alone); there is no standalone `doc`.
#   2. snapshots-current never reads the live website (no network), so it cannot refuse a drifted
#      page, and still refuses a cited page the MANIFEST does not freeze at all.
#   3. the judges' prepare.sh still READ a drifted page (the live copy), and never die on it.
# (A doc-only change re-judging nothing is in subjects.sh: keys and subjects need the engine.)
#
#   .sloprail/tests/docs-follow-recordings.sh      needs sr-checks (SR_CHECKS=path), git, jq, yq, shasum
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SR="${SR_CHECKS:-sr-checks}"
command -v "$SR" >/dev/null && command -v jq >/dev/null && command -v yq >/dev/null || { echo "needs sr-checks, jq and yq" >&2; exit 2; }
T="$(mktemp -d "${TMPDIR:-/tmp}/sr-docs-test.XXXXXX")"; trap 'rm -rf "$T"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
PASS=0; FAIL=0
ok() { PASS=$((PASS + 1)); }
bad() { FAIL=$((FAIL + 1)); echo "FAIL: $1" >&2; }
eq() { [ "$2" = "$3" ] && ok || bad "$1: '$2' != '$3'"; }
sha() { printf '%s' "$1" | shasum -a 256 | cut -d' ' -f1; }
w() { mkdir -p "$(dirname "$1")"; cat >"$1"; }

# a fake curl: the page served is $PAGE (any URL), written to the -o target; every call is logged
mkdir -p "$T/bin"
cat >"$T/bin/curl" <<'SH'
#!/usr/bin/env bash
echo "$*" >>"$CURL_LOG"; out=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done
printf '%s' "$PAGE" >"$out"
SH
chmod +x "$T/bin/curl"
export PATH="$T/bin:$PATH" CURL_LOG="$T/curl.log"; : >"$CURL_LOG"

# --- 1. capture.sh re-freezes the docs its harness's cells cite ---------------------------------
for h in claude codex cursor; do
  R="$T/cap-$h"; mkdir -p "$R"; git -C "$R" init -q -b main
  here="$R/$h-mock/snapshots"; root="$R"; manifest="$here/MANIFEST.yaml"
  die() { echo "capture.sh: $*" >&2; return 1; }
  # the functions of the real script, without its case statement
  eval "$(sed -n '/^capture_doc() {/,/^}/p; /^cited_docs() {/,/^}/p; /^refreeze_cited() {/,/^}/p' "$ROOT/$h-mock/snapshots/capture.sh")"
  PAGE_V1=$'# s\nfirst text\n'; PAGE_V2=$'# s\nsecond text\n'
  w "$manifest" <<EOF2
pin: "1"
docs:
  https://d.example/kept:
    sha256: $(sha "$PAGE_V1")
    fetched: "2000-01-01"
EOF2
  w "$R/spec/capabilities/c.yaml" <<EOF2
statement: c
providers:
  $h:
    docs: [https://d.example/kept#s, https://d.example/new#s]
    runs: [$h-mock/snapshots/runs/r]
  other: {docs: [https://d.example/other#s], runs: [other-mock/snapshots/runs/r]}
EOF2
  w "$R/spec/capabilities/d.yaml" <<<$'statement: d\nproviders:\n  '"$h"$': pending\n'
  eq "capture ($h): cited_docs lists this harness's cited pages, anchor dropped, not another's" "$(cited_docs | tr '\n' ' ')" "https://d.example/kept https://d.example/new "
  PAGE="$PAGE_V1"; export PAGE
  refreeze_cited >/dev/null
  eq "capture ($h): a cited page not yet frozen is frozen with the recording" "$(yq -r '.docs["https://d.example/new"].sha256' "$manifest")" "$(sha "$PAGE_V1")"
  eq "capture ($h): a page already frozen at the live hash is left alone (fetch date too)" "$(yq -r '.docs["https://d.example/kept"].fetched' "$manifest")" "2000-01-01"
  eq "capture ($h): another harness's page is not frozen here" "$(yq -r '.docs["https://d.example/other"] // "none"' "$manifest")" "none"
  PAGE="$PAGE_V2"; export PAGE
  refreeze_cited >/dev/null
  eq "capture ($h): a page that changed upstream is re-frozen at its new hash" "$(yq -r '.docs["https://d.example/kept"].sha256' "$manifest")" "$(sha "$PAGE_V2")"
  [ "$(yq -r '.docs["https://d.example/kept"].fetched' "$manifest")" != "2000-01-01" ] && ok || bad "capture ($h): a re-frozen page gets a new fetch date"
  grep -q 'capture.sh doc\|^  doc)' "$ROOT/$h-mock/snapshots/capture.sh" && bad "capture ($h): no standalone doc re-freeze remains" || ok
  grep -q 'refreeze_cited ;;' "$ROOT/$h-mock/snapshots/capture.sh" && ok || bad "capture ($h): run re-freezes after recording"
done

# --- 2 + 3. snapshots-current and the judges' prepare.sh over a drifted page ---------------------
R="$T/repo"; mkdir -p "$R"; cd "$R"
git init -q -b main .
cp -R "$ROOT/.sloprail" .sloprail; rm -rf .sloprail/tests
git add -A && git commit -q -m base && BASE="$(git rev-parse HEAD)"
FROZEN=$'# s\nfrozen text\n'; DRIFTED=$'# s\nthe page moved on\n'
w claude-mock/snapshots/MANIFEST.yaml <<<$'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: '"$(sha "$FROZEN")"$'\n    fetched: "2026-10-01"'
w claude-mock/snapshots/capture.sh <<<'#!/bin/sh'
w claude-mock/snapshots/runs/r/run.yaml <<<"version: 1"
w claude-mock/snapshots/runs/r/samples/20240101-000000/events.jsonl <<<'{"e":1}'
(cd claude-mock/snapshots/runs/r/samples/20240101-000000 && find . -type f ! -name 'SEAL*' | LC_ALL=C sort | xargs shasum -a 256 >SEAL)
w spec/capabilities/c.yaml <<<$'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r]'
git add -A && git commit -q -m fixture && HEAD="$(git rev-parse HEAD)"

# runcheck RULE SCRIPT — run a rule's check script over its subject's payload
runcheck() {
  local rule="$1" script="$2" payload
  payload="$("$SR" changeset --rule "$rule" --base "$BASE" --head "$HEAD" | jq -c '.subjects[0].payload')"
  printf '%s' "$payload" | SR_TREE="$R" SR_GUARDRAIL_DIR="$R/.sloprail/file-guard/$rule" "$R/.sloprail/file-guard/$rule/$script" 2>&1
  echo "rc=$?"
}
rm -rf .git/sloprail-doc-cache
PAGE="$FROZEN"; export PAGE
OUT="$(runcheck snapshots-current current.sh)"
eq "snapshots-current: a page still at its frozen hash passes" "$OUT" "rc=0"
rm -rf .git/sloprail-doc-cache
PAGE="$DRIFTED"; export PAGE
: >"$CURL_LOG"
OUT="$(runcheck snapshots-current current.sh)"
eq "snapshots-current: a page whose live hash no longer matches its freeze is NOT a refusal" "$OUT" "rc=0"
eq "snapshots-current: ... because it reads no live page at all (no fetch)" "$(wc -c <"$CURL_LOG" | tr -d ' ')" "0"
rm -rf .git/sloprail-doc-cache
for rule in capability-grounded capability-rigor; do
  OUT="$(runcheck $rule prepare.sh)"
  case "$OUT" in *"rc=0"*"live-"*|*"live-"*"rc=0"*) ok ;; *) bad "$rule prepare: a drifted page is read (the live copy), got: $OUT" ;; esac
  case "$OUT" in *"unbound variable"*) bad "$rule prepare: a drifted page must not fail it: $OUT" ;; *) ok ;; esac
done
# control: the same fixture still refuses a cited page the MANIFEST does not freeze at all
w spec/capabilities/c.yaml <<<$'statement: c works\nproviders:\n  claude:\n    docs: [https://d.example/unfrozen#s]\n    runs: [claude-mock/snapshots/runs/r]'
git add -A && git commit -q -m unfrozen && HEAD="$(git rev-parse HEAD)"
OUT="$(runcheck snapshots-current current.sh)"
case "$OUT" in *"cites claude doc 'https://d.example/unfrozen'"*"rc=1"*) ok ;; *) bad "snapshots-current: an unfrozen cited page still refuses, got: $OUT" ;; esac

echo "docs-follow-recordings tests: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
