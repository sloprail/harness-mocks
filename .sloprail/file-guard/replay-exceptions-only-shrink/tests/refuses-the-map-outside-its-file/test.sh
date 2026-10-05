#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves notReplaying is read and written only in its own package files: a mutation from another file, or the map moved to another file, is refused, beside the permit of an emptied list.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker || exit 1
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

good_gen > "$gen"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a generated test reading the list"
BASE=$(git rev-parse HEAD)

# an init() in another file of the package adds an entry the literal does not show
git checkout -q -b init "$BASE"
printf 'package e2e\n\nfunc init() { notReplaying["run-z"] = "flaky: z" }\n' > codex-mock/e2e/001_hooks/zz_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "init adds an entry"
refuses "an init in another file" "is named outside replay_allowlist_test.go"

# the map moved to a file with another name: the old name is gone, the new one is named outside
git rm -q codex-mock/e2e/001_hooks/zz_test.go
git mv "$list" codex-mock/e2e/001_hooks/lists_test.go
git -c user.name=t -c user.email=t@t commit -q -m "rename the list"
refuses "the list renamed" "is named outside replay_allowlist_test.go"

# the list file is gone while the generated test still reads it
git checkout -q -b gone "$BASE"
git rm -q "$list"
git -c user.name=t -c user.email=t@t commit -q -m "drop the list"
refuses "the list gone, still read" "replay_allowlist_test.go is gone or renamed"

# recovery: the list is back, emptied, and the same range passes
printf "$head"'}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "an emptied list"
passes "an emptied list"
