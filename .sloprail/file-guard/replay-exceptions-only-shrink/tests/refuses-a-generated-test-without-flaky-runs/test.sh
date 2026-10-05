#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the refusal of a generated replay test that skips a flaky: entry or runs it once, beside the permit of one that runs it several times.
git init -q .
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

good='const flakyRuns = 3\n\nfunc replayUntilGreen(run func() (string, error), n int) (string, error) { return run() }\n\nfunc f() {\n\tflaky := true\n\tdiff, err = replayUntilGreen(run, flakyRuns)\n\t_ = strings.HasPrefix(reason, "flaky:")\n\tswitch {\n\tcase flaky && (err != nil || diff != ""):\n\t}\n}\n'
printf 'package e2e\n\n'"$good" > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "generated test"
BASE=$(git rev-parse HEAD)

# a flaky: entry that is run once is refused
git checkout -q -b once "$BASE"
sed 's/flakyRuns = 3/flakyRuns = 1/' "$gen" > "$gen.new" && mv "$gen.new" "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run flaky once"
refuses "a flaky: entry run once" "declare 'const flakyRuns = 3'"

# a flaky: entry passed outright, with no retry, is refused
printf 'package e2e\n\nconst flakyRuns = 3\n\nfunc f() {\n\t_ = strings.HasPrefix(reason, "flaky:")\n}\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "no retry"
refuses "a flaky: entry with no retry" "must be run through replayUntilGreen(run, flakyRuns)"

# a block comment holding what the rule asks for, beside a real single run, is refused
printf 'package e2e\n\n/*\n'"$good"'*/\nvar flakyRuns = 1\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "decoy comment"
refuses "a block comment decoy" "a block comment or raw string could hide what this rule reads"

# the same words only in whole-line comments, beside a plain skip, are refused
printf 'package e2e\n\nconst flakyRuns = 3\n\n// func replayUntilGreen(\n// replayUntilGreen(run, flakyRuns)\n// strings.HasPrefix(reason, "flaky:")\n// case flaky && (err != nil || diff != ""):\nfunc f() { t.Skip() }\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "comment only"
refuses "the wiring only in comments" "must be run through replayUntilGreen(run, flakyRuns)"

# the generated test may only read the list: an init() adding an entry is refused
printf 'package e2e\n\n'"$good"'func init() { notReplaying["run-z"] = "flaky: z" }\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "init in the generated test"
refuses "an init in the generated test" "the generated test may only read notReplaying"

# flakyRuns used some other way (here: multiplied away) is refused
printf 'package e2e\n\n'"$good"'func g() int { return flakyRuns * 0 }\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "flakyRuns used elsewhere"
refuses "flakyRuns used elsewhere" "flakyRuns may appear only as"

# recovery: three runs and the retry, and the change passes
printf 'package e2e\n\n'"$good" | sed 's/func f() {/\/\/ ok\nfunc f() {/' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "five runs"
passes "a generated test running a flaky: entry three times"
