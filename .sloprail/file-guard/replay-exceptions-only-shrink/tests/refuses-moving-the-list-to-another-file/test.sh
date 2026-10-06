#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the list may not be moved to a file of another name (the old file is deleted, the new one names the list outside replay_allowlist_test.go), nor named by an init() in another file; the same range with the list kept passes.
git init -q .
mkdir -p codex-mock/e2e/001_hooks
list=codex-mock/e2e/001_hooks/replay_allowlist_test.go
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

git checkout -q -b renamed "$BASE"
git mv "$list" codex-mock/e2e/001_hooks/lists_test.go
git -c user.name=t -c user.email=t@t commit -q -m "rename the list file"
refuses "the list moved to lists_test.go" "notReplaying is named outside replay_allowlist_test.go and generated_replay_test.go"

git checkout -q -b init "$BASE"
printf 'package e2e\n\nfunc init() { notReplaying["run-z"] = "flaky: z" }\n' > codex-mock/e2e/001_hooks/zz_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "init adds an entry from another file"
refuses "an init in another file" "notReplaying is named outside replay_allowlist_test.go and generated_replay_test.go"

# recovery: the list stays where it is, one entry shorter
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git rm -q -f codex-mock/e2e/001_hooks/zz_test.go
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "keep the list, drop an entry"
passes "the list kept in its file, shrunk"
