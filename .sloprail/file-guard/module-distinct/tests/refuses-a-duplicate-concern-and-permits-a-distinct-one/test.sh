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
  jq -es --arg r "$RULE" '[.[] | select(.kind=="FileGuardChecked" and .rule==$r)] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
expect_refused() {   # LABEL SUBSTRING : refused, and the reason says it
  jq -es --arg r "$RULE" --arg s "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule==$r and .outcome=="refused" and (.reason|contains($s)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1: $RULE did not refuse with a reason saying '$2' (sr-checks exit $ran)" >&2; exit 1; }
}
RULE=module-distinct
# the judge is a mock that compares the concerns it is handed (judge.sh)
export SR_CHECKS_JUDGE_MOCKS='{"file-guard/module-distinct/distinct-concern":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
mkdir -p internal/a internal/b
printf 'concern: "a parses config files"\nhome: ["internal/a/**"]\napi: ["internal/a"]\n' >internal/a/module.yaml
printf 'concern: "b renders reports"\nhome: ["internal/b/**"]\napi: ["internal/b"]\n' >internal/b/module.yaml
printf 'package a\n' >internal/a/a.go; printf 'package b\n' >internal/b/b.go
c base; BASE=$(git rev-parse HEAD)
b_concern() { printf 'concern: "%s"\nhome: ["internal/b/**"]\napi: ["internal/b"]\n' "$1" >internal/b/module.yaml; }

# b's concern is changed to what a already owns: the same responsibility, refused
git checkout -q -b duplicate "$BASE"; b_concern "a parses config files"; c "b takes a's concern"
run_rule; expect_refused "a duplicate concern" "the module's concern duplicates another module's"
# the recovery the refusal names: b's concern is sharpened to what only b owns; the same range then passes
b_concern "b renders reports as html"; c "sharpen b's concern"
run_rule; expect_passed "the concern sharpened"
# the nearest neighbour: a concern changed to another one of its own, from the start, passes
git checkout -q -b distinct "$BASE"; b_concern "b renders reports as pdf"; c "b's concern, reworded"
run_rule; expect_passed "a distinct concern"
