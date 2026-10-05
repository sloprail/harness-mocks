#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the generated replay test is read from its syntax tree: shadowing flakyRuns, a dropped retry, wiring only in comments, or a write to the list from inside it are refused, and a good one passes.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker
mkdir -p codex-mock/e2e/001_hooks
list=codex-mock/e2e/001_hooks/replay_allowlist_test.go
gen=codex-mock/e2e/001_hooks/generated_replay_test.go
head='package e2e\n\nvar notReplaying = map[string]string{\n'
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "list"
BASE=$(git rev-parse HEAD)

passes() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1 was not passed by replay-exceptions-only-shrink (sr-checks exit $ran)" >&2; exit 1; }
}
refuses() {
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
  jq -es --arg r "$2" 'any(.[]; .kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink" and .outcome=="refused" and (.reason|contains($r)))' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$1 was not refused with its reason (sr-checks exit $ran)" >&2; exit 1; }
}

good_gen > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "generated test"
BASE=$(git rev-parse HEAD)

# gen_with OLD NEW MESSAGE — the good generated test with OLD replaced by NEW, committed on a fresh branch
n=0
gen_with() {
  n=$((n + 1))
  git checkout -q -b "case$n" "$BASE"
  python3 - "$1" "$2" "$gen" <<'PY'
import sys
old, new, p = sys.argv[1:4]
s = open(p).read()
assert old in s, old
open(p, 'w').write(s.replace(old, new, 1))
PY
  git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$3"
}

gen_with 'const flakyRuns = 3' 'const flakyRuns = 1' "run flaky once"
refuses "a flaky: entry run once" "flakyRuns must be 3"

gen_with 'diff, err = replayUntilGreen(run, flakyRuns)' 'diff, err = run()' "no retry"
refuses "a flaky: entry with no retry" "must run a flaky: entry through replayUntilGreen"

gen_with 'diff, err = replayUntilGreen(run, flakyRuns)' 'diff, err = run() // replayUntilGreen(run, flakyRuns)' "the retry only in a comment"
refuses "the retry only in a comment" "must run a flaky: entry through replayUntilGreen"

gen_with 'diff, err = replayUntilGreen(run, flakyRuns)' 'func(flakyRuns int) { diff, err = replayUntilGreen(run, flakyRuns) }(0)' "a closure shadows flakyRuns"
refuses "a closure shadowing flakyRuns" "must be the package-level const flakyRuns"

gen_with 'diff, err = replayUntilGreen(run, flakyRuns)' 'var flakyRuns int; diff, err = replayUntilGreen(run, flakyRuns)' "a local var shadows flakyRuns"
refuses "a local var shadowing flakyRuns" "is declared again here"

gen_with 'case flaky && (err != nil || diff != ""):' 'case false:' "the failing case is gone"
refuses "no failing case" "a case"

gen_with 't.Errorf("never green in %d runs, see notReplaying", flakyRuns)' 't.Errorf("x", func() int { notReplaying["z"] = "flaky: z"; return 0 }())' "a write inside a t.Errorf call"
refuses "a write inside a t.Errorf call" "the generated test may only read notReplaying"

gen_with 't.Errorf("never green in %d runs, see notReplaying", flakyRuns)' 't.Errorf("x", flakyRuns); notReplaying["z"] = "y"' "a write after a t.Errorf"
refuses "a write after a t.Errorf" "the generated test may only read notReplaying"

gen_with 'for name := range notReplaying {' 'delete(notReplaying, "a"); for name := range notReplaying {' "a delete"
refuses "a delete" "the generated test may only read notReplaying"

# recovery: the good test again, with one more read of the list, and the same range passes
gen_with '_ = name' '_, _ = name, notReplaying["a"]' "another read"
passes "a generated test that only reads the list"
