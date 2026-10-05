#!/usr/bin/env bash
# Rule tests for what the judge prompts demand and what the deterministic rules refuse, through the
# engine's real interface: a repository is built, the rule's payload is what `sr-checks changeset`
# says the rule is handed, and the rule's own script is run on it with the environment the engine
# gives it.
#
#   .sloprail/tests/rules.sh      needs sr-checks (SR_CHECKS=path), git, jq, yq, shasum
#
# A judge cannot be run here (it asks a model), so for the judge templates what is covered is that
# the template carries the clause the verdict depends on.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SR="${SR_CHECKS:-sr-checks}"
command -v "$SR" >/dev/null && command -v jq >/dev/null && command -v yq >/dev/null || { echo "needs sr-checks, jq and yq" >&2; exit 2; }
T="$(mktemp -d "${TMPDIR:-/tmp}/sr-rules-test.XXXXXX")"; trap 'rm -rf "$T"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
PASS=0; FAIL=0
ok() { PASS=$((PASS + 1)); }
bad() { FAIL=$((FAIL + 1)); echo "FAIL: $1" >&2; }
eq() { [ "$2" = "$3" ] && ok || bad "$1: '$2' != '$3'"; }
has() { grep -qF -e "$3" <<<"$2" && ok || bad "$1: no '$3' in: $2"; }

# --- (a) every judge template asks for EVERY failure in one verdict -----------------------------
for f in "$ROOT"/.sloprail/file-guard/*/*.md.j2 "$ROOT"/.sloprail/_lib/*.md.j2; do
  grep -q '^## Every failure, in one verdict' "$f" && grep -q 'lists EVERY failing item' "$f" && ok || bad "$f lacks the 'Every failure, in one verdict' section"
done

# --- (c) (d) the grounded judge fails an undisclosed conflict and a negated defining clause -----
G="$ROOT/.sloprail/file-guard/capability-grounded/docs-support-statement.md.j2"
has "grounded judge: conflict disclosure" "$(cat "$G")" 'Doc and'
has "grounded judge: conflict disclosure fails" "$(cat "$G")" 'A conflict nobody disclosed is a failure'
has "grounded judge: negated defining clause" "$(cat "$G")" 'A deviation never negates what defines the cell'
has "grounded judge: negation means unsupported" "$(cat "$G")" 'must be `supported: false`'
has "ADR: conflict disclosed" "$(cat "$ROOT/adr/capability-grounding/ADR.md")" 'Doc and'
has "ADR: defining clause" "$(cat "$ROOT/adr/capability-grounding/ADR.md")" 'defining clause'
has "ADR: negation is supported: false" "$(cat "$ROOT/adr/capability-grounding/ADR.md")" '`supported: false`'

R="$T/repo"; mkdir -p "$R"; cd "$R"
git init -q -b main .
cp -R "$ROOT/.sloprail" .sloprail; rm -rf .sloprail/tests
w() { mkdir -p "$(dirname "$1")"; cat >"$1"; }
step() { git add -A && git commit -q -m "$1" && git rev-parse HEAD; }
# payload RULE BASE HEAD ID: what the engine hands RULE's checks for subject ID ("" : the whole changeset)
payload() {
  if [ -n "$4" ]; then "$SR" changeset --rule "$1" --base "$2" --head "$3" | jq -c --arg id "$4" '.subjects[] | select(.id == $id) | .payload'
  else "$SR" changeset --rule "$1" --base "$2" --head "$3" | jq -c '.payload'; fi
}
# runcheck RULE SCRIPT BASE HEAD ID: the script's exit status (its output goes to $T/out)
runcheck() {
  payload "$1" "$3" "$4" "$5" | SR_TREE="$R" SR_GUARDRAIL_DIR="$R/.sloprail/file-guard/$1" "$R/.sloprail/file-guard/$1/$2" >"$T/out" 2>&1; echo $?
}

# --- (b) a new deviation, or a new absence claim, needs the user's words (added-or-removed.sh) ---
DEV_Y=$'    deviations:\n      - adr: modeled-surface\n        statement: The mock does not do y.'
DEV_YZ=$'    deviations:\n      - adr: modeled-surface\n        statement: The mock does not do y, nor z.'
DEV_CONFLICT=$'    deviations:\n      - adr: capability-grounding\n        statement: \'Doc and recording conflict: the doc says y, the recording shows z.\''
cap() {   # cap CELL-EXTRA: the capability x, claude cites a doc and a run, plus CELL-EXTRA
  { printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/x#s]\n    runs: [claude-mock/snapshots/runs/rx]\n'
    [ -z "$1" ] || printf '%s\n' "$1"
    printf '  codex: pending\n'; } | w spec/capabilities/x.yaml
}
cap ""; BASE="$(step base)"
when() { eq "when: $1" "$(runcheck capability-grounded added-or-removed.sh "$2" "$3" x)" "$4"; }   # 0 = words required, 1 = waived
cap "$DEV_Y"; H1="$(step new-deviation)"
when "a new deviation requires the user's words" "$BASE" "$H1" 0
git checkout -q "$BASE"; cap "$DEV_CONFLICT"; H2="$(step conflict-disclosure)"
when "a disclosed doc/recording conflict needs none" "$BASE" "$H2" 1
git checkout -q "$BASE"
w spec/capabilities/x.yaml <<'EOF'
statement: x works
providers:
  claude: {supported: false, reason: the harness lacks x, runs: [claude-mock/snapshots/runs/rx]}
  codex: pending
EOF
H3="$(step becomes-unsupported)"
when "a cell that becomes unsupported requires the user's words" "$BASE" "$H3" 0
git checkout -q "$BASE"
w spec/capabilities/x.yaml <<'EOF'
statement: x works
providers:
  claude:
    docs: [https://d.example/x#s, https://d.example/x#t]
    runs: [claude-mock/snapshots/runs/rx]
  codex: pending
EOF
H4="$(step more-docs)"
when "a change to a cell's docs needs none" "$BASE" "$H4" 1
git checkout -q "$BASE"; cap "$DEV_Y"; D1="$(step with-deviation)"
cap ""; D2="$(step dropped-deviation)"
when "dropping a deviation needs none" "$D1" "$D2" 1
cap "$DEV_YZ"; D3="$(step reworded-deviation)"
when "rewording a deviation that stood requires the user's words" "$D1" "$D3" 0

# --- (e) a recording must be clean and in the mode the mock imitates (snapshots-current) ---------
git checkout -q "$BASE"
seal() { (cd "$1" && find . -type f ! -name 'SEAL*' | LC_ALL=C sort | xargs shasum -a 256 >SEAL); }
# run NAME COMMAND PAYLOADS STDERR STREAM [ARGS]: a sealed, otherwise valid run of codex-mock
run() {
  local d="codex-mock/snapshots/runs/$1" s="codex-mock/snapshots/runs/$1/samples/20240101-000000"
  rm -rf "$d"; mkdir -p "$d/setup" "$s"
  printf 'version: 0.1.0\ncommand: %s\n' "$2" >"$d/run.yaml"; : >"$d/setup/prompt.txt"
  [ -z "${6:-}" ] || printf '%s\n' "$6" >"$d/setup/args"
  printf '{"e":1}\n' >"$s/events.jsonl"; printf '%s' "$3" >"$s/payloads.jsonl"; printf '%s' "$4" >"$s/stderr.txt"; printf '%s' "$5" >"$s/stream.jsonl"; : >"$s/exit.txt"
  seal "$s"
}
GOODCMD='codex exec --json --skip-git-repo-check -m m'
cites() { w spec/capabilities/x.yaml <<EOF
statement: x works
providers:
  codex: {supported: false, reason: the harness lacks x, runs: [codex-mock/snapshots/runs/$1]}
EOF
}
w codex-mock/snapshots/MANIFEST.yaml <<<"pin: 0.1.0"; w codex-mock/snapshots/capture.sh <<<"#!/bin/sh"
BASE="$(step snapshots-base)"
cur() {   # cur NAME COMMAND PAYLOADS STDERR STREAM [ARGS]: snapshots-current's refusal text for that one run ("" when it passes)
  run "$@"; cites "$1"; local h rc; h="$(step "run $1")"
  rc="$(runcheck snapshots-current current.sh "$BASE" "$h" codex)"
  if [ "$rc" = 0 ]; then echo ""; else cat "$T/out"; fi
  git reset -q --hard "$BASE"
}
eq "recording: a clean run passes" "$(cur clean "$GOODCMD" $'{"a":1}\n{"b":2}\n' $'Reading additional input from stdin...\n' $'{"type":"thread.started"}\n')" ""
has "recording: a non-JSON line in the hook log is refused" "$(cur unparsed "$GOODCMD" $'{"a":1}\nnot json at all\n' "" "")" "not JSON"
has "recording: a harness error line on stderr is refused" "$(cur err-stderr "$GOODCMD" "" $'2026-01-01T00:00:00Z ERROR codex_core::session: boom\n' "")" "error line to stderr"
has "recording: an error frame in the stream is refused" "$(cur err-stream "$GOODCMD" "" "" $'{"type":"turn.failed","error":{"message":"x"}}\n')" "error frame"
has "recording: a run recorded without the structured-output flag is refused" "$(cur mode-text 'codex exec --skip-git-repo-check -m m' "" "" "")" "lacks '--json'"
has "recording: a run that switches the mode on in setup/args is refused" "$(cur mode-v2 "$GOODCMD" "" "" "" "--enable")" "switches the mode with --enable"
# declared in expected.yaml (the rule's own file): it passes
printf '%s\n' "codex/unparsed: {unparsed: 1}" "codex/err-stderr: {stderr: ' ERROR codex_core::session: boom\$'}" "codex/mode-v2: {mode: a variant on purpose}" >>.sloprail/file-guard/snapshots-current/expected.yaml
BASE="$(step declare)"
eq "recording: a declared non-JSON line passes" "$(cur unparsed "$GOODCMD" $'{"a":1}\nnot json at all\n' "" "")" ""
eq "recording: a declared stderr line passes" "$(cur err-stderr "$GOODCMD" "" $'2026-01-01T00:00:00Z ERROR codex_core::session: boom\n' "")" ""
eq "recording: a declared mode variant passes" "$(cur mode-v2 "$GOODCMD" "" "" "" "--enable")" ""
has "recording: more non-JSON lines than declared is refused" "$(cur unparsed "$GOODCMD" $'{"a":1}\nnot json\nand more\n' "" "")" "has 2 line(s)"

# capture.sh must not seal a sample the rule would refuse: each harness's script runs the same check
for h in claude codex cursor; do
  c="$ROOT/$h-mock/snapshots/capture.sh"
  grep -q '_lib/recording.sh' "$c" && grep -q "recording_problems $h " "$c" && ok || bad "$h-mock/snapshots/capture.sh does not run recording_problems before sealing"
done

echo "rule tests: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ]
