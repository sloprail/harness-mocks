#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the refusal of an entry the rule cannot read (a trailing comment, a one-line map), which would otherwise hide a weakened or added entry.
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

# a weakened entry with a trailing comment is not read as an entry: refused, not passed
git checkout -q -b hides "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "flaky: y", // why\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b is flaky, commented"
refuses "a flaky entry with a trailing comment" "is not one \"run\": \"reason\", entry"

# the whole map on one line hides every entry, an added one too
printf 'package e2e\n\nvar notReplaying = map[string]string{"run-a": "adapter: x", "run-c": "mock gap: z"}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "one-line map"
refuses "a one-line map" "the notReplaying map could not be found"

# recovery: one entry per line, and the list only shrank
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
passes "the list shrunk from its base"
