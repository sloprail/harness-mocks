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

# a commented copy of the header beside a one-line real map hides the real entries too
printf 'package e2e\n\n/*\nvar notReplaying = map[string]string{\n}\n*/\nvar notReplaying = map[string]string{"run-a": "adapter: x", "run-d": "flaky: z"}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "commented header"
refuses "a commented header beside a one-line map" "notReplaying must be declared exactly once"

# a reason whose category is written with an escape is read by Go as another one: refused
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
printf "$head"'\t"run-a": "\\u0066laky: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "escaped flaky"
refuses "an escaped flaky: reason" "is not one \"run\": \"reason\", entry"

# an entry added by an init() is an entry the literal does not show: refused
printf "$head"'\t"run-a": "adapter: x",\n}\n\nfunc init() { notReplaying["run-e"] = "flaky: z" }\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "init adds an entry"
refuses "an init adding an entry" "notReplaying may appear only on its declaration line"

# a key with a tab in it would break the key and rank columns the rule compares by: refused
git checkout -q -b tabkey "$BASE"
printf "$head"'\t"run\ta": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a key with a tab"
refuses "a key with a tab" "is not one \"run\": \"reason\", entry"

# recovery: one entry per line, and the list only shrank
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
passes "the list shrunk from its base"
