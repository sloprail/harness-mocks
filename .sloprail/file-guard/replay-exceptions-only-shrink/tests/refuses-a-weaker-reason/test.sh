#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the refusal of an existing entry whose reason moves to a weaker category (untriaged:/adapter: to flaky:), beside the permit of a stronger one.
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

# untriaged: becomes flaky:, and the rule refuses it, naming the entry
git checkout -q -b weakens "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "flaky: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b is flaky"
refuses "untriaged -> flaky for run-b" "run-b (untriaged: -> flaky:)"

# adapter: becomes flaky: as well
printf "$head"'\t"run-a": "flaky: x",\n\t"run-b": "flaky: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-a is flaky"
refuses "adapter -> flaky for run-a" "run-a (a triaged reason -> flaky:)"

# recovery: the original reasons, then a reason triaged (untriaged: becomes mock gap:) passes
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "mock gap: triaged",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b is triaged"
passes "a triaged reason"
