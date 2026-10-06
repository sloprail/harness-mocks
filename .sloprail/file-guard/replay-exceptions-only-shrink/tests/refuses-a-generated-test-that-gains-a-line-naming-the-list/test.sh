#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the generated replay test may not gain a line that names notReplaying (an init() writing an entry, a delete(, a maps.Copy(), while it keeps its reads of the list; a commented line is not one.
git init -q .
mkdir -p claude-mock/e2e/018_replay
list=claude-mock/e2e/018_replay/replay_allowlist_test.go
gen=claude-mock/e2e/018_replay/generated_replay_test.go
head='package e2e\n\nvar notReplaying = map[string]string{\n'
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
printf 'package e2e\n\nfunc TestG() {\n\tfor name := range notReplaying {\n\t\t_ = name\n\t}\n}\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "list and generated test"
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

# an init() in the generated test adds an entry the list does not show: refused
git checkout -q -b init "$BASE"
printf 'package e2e\n\nfunc TestG() {\n\tfor name := range notReplaying {\n\t\t_ = name\n\t}\n}\n\nfunc init() { notReplaying["zz"] = "adapter: x" }\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "init adds an entry"
refuses "an init in the generated test" "the generated test names notReplaying in a line that was not there before"

# a delete( is refused the same way
git checkout -q -b del "$BASE"
printf 'package e2e\n\nfunc TestG() {\n\tfor name := range notReplaying {\n\t\t_ = name\n\t}\n\tdelete(notReplaying, "run-a")\n}\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "delete from the list"
refuses "a delete from the list" "the generated test names notReplaying in a line that was not there before"

# and so is a maps.Copy(
git checkout -q -b copy "$BASE"
printf 'package e2e\n\nfunc TestG() {\n\tfor name := range notReplaying {\n\t\t_ = name\n\t}\n\tmaps.Copy(notReplaying, other)\n}\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "maps.Copy into the list"
refuses "a maps.Copy into the list" "the generated test names notReplaying in a line that was not there before"

# recovery: a comment naming the list, and the reads it had, and the same range from the same base passes
git checkout -q -b ok "$BASE"
printf 'package e2e\n\n// notReplaying is read here, never written\nfunc TestG() {\n\tfor name := range notReplaying {\n\t\t_ = name\n\t}\n}\n' > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a comment"
passes "the generated test with only a comment naming the list"
