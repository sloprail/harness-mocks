#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the refusal of an added entry, beside the permit of a shrinking list.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker
mkdir -p codex-mock/e2e/001_hooks
list=codex-mock/e2e/001_hooks/replay_allowlist_test.go
head='package e2e\n\nvar notReplaying = map[string]string{\n'
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "list"
BASE=$(git rev-parse HEAD)

# an entry is removed, then the last one: the list shrinks to empty, and the rule permits it
git checkout -q -b shrinks "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
printf "$head"'}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-a replays"
: > "$SR_EVENTS_FILE"
sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "a shrinking list was not passed by replay-exceptions-only-shrink (sr-checks exit $ran)" >&2; exit 1; }

# a third entry is added, and the rule refuses it, naming it
git checkout -q -b grows "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n\t"run-c": "untriaged: z",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "add an entry"
: > "$SR_EVENTS_FILE"
sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es 'any(.[]; .kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink" and .outcome=="refused" and (.reason|contains("the replay exception list may only shrink, and these entries were added: run-c")))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the added entry run-c was not refused with its reason (sr-checks exit $ran)" >&2; exit 1; }

# recovery: the run replays (and so does run-b), so the entries are removed, and the same range from the same base passes
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-c and run-b replay"
: > "$SR_EVENTS_FILE"
sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "the list back to its base was not passed by replay-exceptions-only-shrink (sr-checks exit $ran)" >&2; exit 1; }
