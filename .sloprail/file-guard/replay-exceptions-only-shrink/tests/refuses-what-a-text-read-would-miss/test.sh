#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the list is read as Go reads it, not as text: a comment, a one-line map, an escape or a commented header cannot hide a weakened or added entry.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker
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

# a weakened entry with a trailing comment: read as the weakening it is
git checkout -q -b comment "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "flaky: y", // why\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b is flaky, commented"
refuses "a flaky entry with a trailing comment" "run-b (untriaged: -> flaky:)"

# an escaped category is the category Go decodes
git checkout -q -b escape "$BASE"
printf "$head"'\t"run-a": "\\u0066laky: x",\n\t"run-b": "untriaged: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "escaped flaky"
refuses "an escaped flaky: reason" "run-a (a triaged reason -> flaky:)"

# the whole map on one line: its added entry is seen
git checkout -q -b oneline "$BASE"
printf 'package e2e\n\nvar notReplaying = map[string]string{"run-a": "adapter: x", "run-b": "untriaged: y", "run-c": "mock gap: z"}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "one-line map"
refuses "a one-line map" "these entries were added: run-c"

# a commented copy of the header beside a one-line real map hides nothing
git checkout -q -b decoy "$BASE"
printf 'package e2e\n\n/*\nvar notReplaying = map[string]string{\n}\n*/\nvar notReplaying = map[string]string{"run-a": "adapter: x", "run-b": "untriaged: y", "run-d": "flaky: z"}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "commented header"
refuses "a commented header" "these entries were added: run-d"

# an entry added by an init() in the list file is refused: the map is named only by its declaration
git checkout -q -b init "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n\nfunc init() { notReplaying["run-e"] = "flaky: z" }\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "init adds an entry"
refuses "an init adding an entry" "is named 2 times in the file"

# recovery: one entry per line, and the list only shrank
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
passes "the list shrunk from its base"
