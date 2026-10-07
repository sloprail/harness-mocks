#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. gofmt writes an empty map on one line, `map[string]string{}`: that is the empty
# list, the goal, and the rule reads it as one. Emptying a list to `{}` passes; adding an entry to a `{}` list is
# refused, naming the entry; and the list back to `{}` passes.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker || exit 1
mkdir -p claude-mock/e2e/018_replay
list=claude-mock/e2e/018_replay/replay_allowlist_test.go
head='package e2e\n\nvar notReplaying = map[string]string{'
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
expect_passed() {   # BASE LABEL
  : > "$SR_EVENTS_FILE"
  sr-checks run --base "$1" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
  jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
    { jq -c . "$SR_EVENTS_FILE" >&2; echo "$2: replay-exceptions-only-shrink did not pass (sr-checks exit $ran)" >&2; exit 1; }
}
printf "$head"'\n\t"run-a": "adapter: x",\n}\n' > "$list"; c "list"
BASE=$(git rev-parse HEAD)

# the list is emptied and gofmt writes it on one line: the rule permits it
git checkout -q -b empties "$BASE"
printf 'package e2e\n\nvar notReplaying = map[string]string{}\n' > "$list"; c "run-a replays"
expect_passed "$BASE" "a list emptied to {}"
EMPTY=$(git rev-parse HEAD)

# an entry is added back to the empty one-line map: refused, naming the entry
git checkout -q -b regrows "$EMPTY"
printf "$head"'\n\t"run-b": "untriaged: y",\n}\n' > "$list"; c "run-b is listed"
: > "$SR_EVENTS_FILE"
sr-checks run --base "$EMPTY" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es 'any(.[]; .kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink" and .outcome=="refused" and (.reason|contains("the replay exception list may only shrink, and these entries were added: run-b")))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "an entry added to a {} list was not refused with its reason (sr-checks exit $ran)" >&2; exit 1; }

# recovery: the entry goes and the list is empty again, in gofmt-less two-line form (the same list), and the same range from the same base passes
printf 'package e2e\n\nvar notReplaying = map[string]string{\n}\n' > "$list"; c "run-b replays"
expect_passed "$BASE" "the list back to {}, against the list it started as"
