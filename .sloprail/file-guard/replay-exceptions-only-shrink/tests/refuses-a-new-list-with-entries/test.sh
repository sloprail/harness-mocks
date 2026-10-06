#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves a new replay exception list (a mock's first) may carry no entries, and may be empty.
git init -q .
mkdir -p claude-mock/e2e/018_replay
list=claude-mock/e2e/018_replay/replay_allowlist_test.go
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

git checkout -q -b newlist "$BASE"
mkdir -p cursor-mock/e2e/001_hooks
new=cursor-mock/e2e/001_hooks/replay_allowlist_test.go
printf "$head"'\t"run-x": "untriaged: y",\n}\n' > "$new"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a new list with an entry"
refuses "a new list carrying run-x" "these entries were added: run-x"

# recovery: the new list is empty, and the same range passes
printf "$head"'}\n' > "$new"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "an empty new list"
passes "an empty new list"
