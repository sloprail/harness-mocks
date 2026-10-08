#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the refusal of an entry added to the claude list (the rule covers every mock's list, not one path), and its recovery.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker || exit 1
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

git checkout -q -b grows "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n\t"run-c": "mock gap: z",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "add an entry"
refuses "the added claude entry run-c" "the replay exception list may only shrink, and these entries were added: run-c"

# a new flaky: entry is an addition too
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n\t"run-d": "flaky: z",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "add a flaky entry"
refuses "the added flaky claude entry run-d" "these entries were added: run-d"

# recovery: the added entries are gone and run-b replays too, so the same range from the same base passes
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b and run-d replay"
passes "the list shrunk from its base"
