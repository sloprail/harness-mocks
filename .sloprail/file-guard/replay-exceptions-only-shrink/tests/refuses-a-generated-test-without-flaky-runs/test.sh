#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the generated replay test is pinned: replayUntilGreen and TestGeneratedReplay must equal their canonical copies (comments aside), so a changed loop, a changed case, a stub or a shadowed flakyRuns is refused, and so is a write to the list; an unchanged file, a commented one, passes.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker || exit 1
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

gen_with 'i < attempts &&' 'i < 1 &&' "a changed loop"
refuses "a changed loop" "replayUntilGreen differs from the canonical copy"

gen_with $'\t\tdiff, err = run()\n' $'\t\t_, _ = run()\n' "the loop discards run()'s result"
refuses "a loop discarding run()'s result" "replayUntilGreen differs from the canonical copy"

gen_with $'\tdiff, err := run()\n\tfor i := 1; i < attempts && (err != nil || diff != ""); i++ {\n\t\tdiff, err = run()\n\t}\n\treturn diff, err' $'\treturn run()' "a stub replayUntilGreen"
refuses "a stub replayUntilGreen" "replayUntilGreen differs from the canonical copy"

gen_with 'case flaky && (err != nil || diff != ""):' 'case flaky && false:' "a changed case"
refuses "a changed case" "TestGeneratedReplay differs from the canonical copy"

gen_with $'\t\t\tswitch {\n' $'\t\t\tswitch {\n\t\t\tcase true:\n' "an always-true case first"
refuses "an always-true case first" "TestGeneratedReplay differs from the canonical copy"

gen_with 'diff, err = replayUntilGreen(run, flakyRuns)' 'func(flakyRuns int) { diff, err = replayUntilGreen(run, flakyRuns) }(0)' "a closure shadows flakyRuns"
refuses "a closure shadowing flakyRuns" "TestGeneratedReplay differs from the canonical copy"

gen_with 'flaky := listed && strings.HasPrefix(reason, "flaky:")' 'flaky := false' "an always-false flaky define"
refuses "an always-false flaky define" "TestGeneratedReplay differs from the canonical copy"

gen_with 'func TestGeneratedReplay(' $'func init() { notReplaying["z"] = "flaky: z" }\n\nfunc TestGeneratedReplay(' "an init writing to the list"
refuses "an init in the generated test" "the generated test may only read notReplaying"

gen_with 'func TestGeneratedReplay(' $'func init() { (notReplaying["z"]) = "flaky:y" }\n\nfunc TestGeneratedReplay(' "a parenthesised write"
refuses "a parenthesised write to the list" "the generated test may only read notReplaying"

# recovery: the canonical code again, with comments and one more read of the list, and the same range passes
gen_with 'func TestGeneratedReplay(' $'// a comment does not change the code\nfunc readOne() string { return notReplaying["a"] }\n\nfunc TestGeneratedReplay(' "comments and a read"
passes "the canonical generated test, commented, with a read of the list"
