#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves a list moved to another replay directory of its mock is compared with its old self: the same entries (a rename), fewer entries (a rewrite, so a delete and an add), and a recovery.
git init -q .
mkdir -p codex-mock/e2e/001_hooks codex-mock/e2e/002_replay
old=codex-mock/e2e/001_hooks/replay_allowlist_test.go
new=codex-mock/e2e/002_replay/replay_allowlist_test.go
head='package e2e\n\nvar notReplaying = map[string]string{\n'
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n' > "$old"
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

# the same entries, moved: permitted
git checkout -q -b move "$BASE"
mkdir -p codex-mock/e2e/002_replay
git mv "$old" "$new"
git -c user.name=t -c user.email=t@t commit -q -m "move the list"
git diff --name-status -M "$BASE" HEAD | grep -q "^R" || { echo "the pure move is not a rename to git" >&2; exit 1; }
passes "a list moved with the same entries"

# moved and rewritten (too different for git to call it a rename), with an entry fewer: permitted
git checkout -q -b rewrite "$BASE"
mkdir -p codex-mock/e2e/002_replay
git rm -q "$old"
printf "// xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n// xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n// xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n// xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n// xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n// xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n"'package e2e\n\nvar notReplaying = map[string]string{\n\t"run-a": "adapter: x",\n}\n' > "$new"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "move and shrink the list"
git diff --name-status -M "$BASE" HEAD | grep -q "^D" || { echo "the rewrite is a rename to git, not a delete and an add" >&2; exit 1; }
passes "a list moved and shrunk"
