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

good='const flakyRuns = 3\n\nfunc f() {\n\tflaky := true\n\tdiff, err = replayUntilGreen(run, flakyRuns)\n\t_ = strings.HasPrefix(reason, "flaky:")\n\tswitch {\n\tcase flaky && (err != nil || diff != ""):\n\t}\n}\n'
printf 'package e2e\n\n'"$good" > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "generated test"
BASE=$(git rev-parse HEAD)

# a flaky: entry that is run once is refused
git checkout -q -b once "$BASE"
sed 's/flakyRuns = 3/flakyRuns = 1/' "$gen" > "$gen.new" && mv "$gen.new" "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run flaky once"
refuses "a flaky: entry run once" "declare 'const flakyRuns = N' with N of at least 2"

# a flaky: entry passed outright, with no retry, is refused
printf 'package e2e\n\nconst flakyRuns = 3\n\nfunc f() {\n\t_ = strings.HasPrefix(reason, "flaky:")\n}\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "no retry"
refuses "a flaky: entry with no retry" "must be run through replayUntilGreen(run, flakyRuns)"

# recovery: the runs are 5, still several, and the change passes
printf 'package e2e\n\n'"$good" | sed 's/flakyRuns = 3/flakyRuns = 5/' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "five runs"
passes "a generated test running a flaky: entry five times"
