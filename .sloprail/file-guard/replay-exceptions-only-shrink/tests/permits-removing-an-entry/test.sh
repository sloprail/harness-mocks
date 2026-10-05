#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges a committed range with the project's rules; only this rule's outcome is asserted.
git init -q .
mkdir -p codex-mock/e2e/001_hooks
list=codex-mock/e2e/001_hooks/replay_allowlist_test.go
printf 'package e2e\n\nvar notReplaying = map[string]string{\n\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "list"
BASE=$(git rev-parse HEAD)

# an entry is removed, then the last one: the list shrinks to empty
printf 'package e2e\n\nvar notReplaying = map[string]string{\n\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
printf 'package e2e\n\nvar notReplaying = map[string]string{\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-a replays"

set +e
sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1
set -e
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; echo "a shrinking list was not passed by replay-exceptions-only-shrink" >&2; exit 1; }
