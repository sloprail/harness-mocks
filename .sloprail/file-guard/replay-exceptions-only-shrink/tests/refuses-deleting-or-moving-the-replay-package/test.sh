#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. The replay test package is pinned whole: deleting it, deleting its generated
# test or its list, or renaming the package directory ends the replay (and the flaky: repetition) without removing
# an entry, and is refused; removing an entry from the list still passes.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker || exit 1
pkg=codex-mock/e2e/001_hooks
mkdir -p "$pkg"
list=$pkg/replay_allowlist_test.go
gen=$pkg/generated_replay_test.go
head='package e2e\n\nvar notReplaying = map[string]string{\n'
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n' > "$list"
good_gen > "$gen"
printf 'package e2e\n' > "$pkg/main_test.go"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "a replay package"
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

# the whole package is deleted: no list, no generated test, nothing left to judge
git checkout -q -b delete-package "$BASE"
git rm -q -r "$pkg"
git -c user.name=t -c user.email=t@t commit -q -m "drop the replay package"
refuses "the replay package deleted" "the replay test package is pinned"

# the generated test alone is deleted (its repetition and its reads go)
git checkout -q -b delete-generated "$BASE"
git rm -q "$gen"
git -c user.name=t -c user.email=t@t commit -q -m "drop the generated test"
refuses "the generated test deleted" "$gen: the replay test package is pinned"

# the package directory is renamed
git checkout -q -b rename-package "$BASE"
git mv "$pkg" codex-mock/e2e/002_replay
git -c user.name=t -c user.email=t@t commit -q -m "rename the package"
refuses "the package directory renamed" "the replay test package is pinned"

# boundary: an entry removed from the list, the package where it was, still passes
git checkout -q -b shrinks "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
passes "a list that lost an entry"
