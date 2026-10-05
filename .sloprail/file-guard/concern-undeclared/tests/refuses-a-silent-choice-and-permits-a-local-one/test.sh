#!/usr/bin/env bash
set -euo pipefail

# A rule refuses when it cannot work out what to check; it never passes on a failed lookup (#204).
# The CI path, no agent turn: `sr-checks run` judges a committed range with the project's rules. Each
# lookup of the rule is made to FAIL in turn by a `jq` shim put first on PATH for that run only: it exits 3
# when an argument equals $FAIL_JQ_EXACT or contains $FAIL_JQ_HAS (a piece of the lookup's own program),
# else it runs the real jq. The rule must then refuse, saying which lookup failed. A control run, with no
# failure injected, shows the same range is otherwise judged and passes.
# The verdicts are kept by content, so every injection is its own commit (a stored verdict is replayed).
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" "$TMPDIR/shim" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"
REAL_JQ="$(command -v jq)"
cat >"$TMPDIR/shim/jq" <<SHIM
#!/usr/bin/env bash
for a in "\$@"; do
  [ -n "\${FAIL_JQ_EXACT:-}" ] && [ "\$a" = "\$FAIL_JQ_EXACT" ] && exit 3
  [ -n "\${FAIL_JQ_HAS:-}" ] && case "\$a" in *"\$FAIL_JQ_HAS"*) exit 3 ;; esac
done
exec "$REAL_JQ" "\$@"
SHIM
chmod +x "$TMPDIR/shim/jq"
git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
# run_rule [EXACT HAS] — judge BASE..HEAD, the jq failing when asked to; the rule's events land in $SR_EVENTS_FILE
run_rule() {
  : > "$SR_EVENTS_FILE"
  PATH="$TMPDIR/shim:$PATH" FAIL_JQ_EXACT="${1:-}" FAIL_JQ_HAS="${2:-}" sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
}
expect_passed() {   # LABEL
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="concern-undeclared")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="concern-undeclared" and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
RULE=concern-undeclared
# the judge is a mock that reads the diffs it is pointed at (judge.sh)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/concern-undeclared/classify":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
mkdir -p adr/one internal/a
printf -- '---\nconcern: one thing\nsloprails: []\n---\n# One\n' >adr/one/ADR.md
printf 'concern: "a owns a"\nhome: ["internal/a/**"]\napi: ["internal/a"]\n' >internal/a/module.yaml
printf 'package a\n' >internal/a/a.go
c base; BASE=$(git rev-parse HEAD)

# a change that stamps records with time.Now() directly settles the time source in passing: refused
git checkout -q -b silent "$BASE"
printf 'package a\n\nfunc stamp() int64 { return time.Now().Unix() }\n' >internal/a/a.go; c "stamp with the clock"
run_rule; expect_refused "a silent choice" "No ADR decides this"
# the recovery: the clock is passed in (a local choice that sets nothing for others); the same range then passes
printf 'package a\n\nfunc stamp(now func() int64) int64 { return now() }\n' >internal/a/a.go; c "take the clock as an argument"
run_rule; expect_passed "the clock passed in"
# the nearest neighbour: an unrelated local change from the start passes
git checkout -q -b local "$BASE"
printf 'package a\n\nfunc double(n int) int { return n * 2 }\n' >internal/a/a.go; c "a local helper"
run_rule; expect_passed "a local change"
