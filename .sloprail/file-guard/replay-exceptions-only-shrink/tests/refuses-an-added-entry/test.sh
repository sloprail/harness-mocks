#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. Proves the refusal of an added entry, beside the permit of a shrinking list.
git init -q .
mkdir -p codex-mock/e2e/001_hooks
list=codex-mock/e2e/001_hooks/replay_allowlist_test.go
head='package e2e\n\nvar notReplaying = map[string]string{\n'
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "list"
BASE=$(git rev-parse HEAD)

# verdict: this rule's outcome(s) over the range, "passed", "refused: <reason>" or "none"
verdict() {
  : > "$SR_EVENTS_FILE"
  set +e
  sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1
  set -e
  jq -r '[.[] | select(.kind=="FileGuardChecked" and .rule=="replay-exceptions-only-shrink")] | if length == 0 then "none" elif all(.[]; .outcome=="passed") then "passed" else "refused: " + (map(.reason // "") | join(" ")) end' < <(jq -s . "$SR_EVENTS_FILE")
}

# an entry is removed, then the last one: the list shrinks to empty, and the rule permits it
git checkout -q -b shrinks "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-b replays"
printf "$head"'}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "run-a replays"
got="$(verdict)"
[ "$got" = passed ] || { echo "a shrinking list: want passed, got: $got" >&2; exit 1; }

# a third entry is added, and the rule refuses it, naming it
git checkout -q -b grows "$BASE"
printf "$head"'\t"run-a": "adapter: x",\n\t"run-b": "untriaged: y",\n\t"run-c": "untriaged: z",\n}\n' > "$list"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "add an entry"
got="$(verdict)"
case "$got" in
  *"the replay exception list may only shrink, and these entries were added: run-c"*) ;;
  *) echo "an added entry: want a refusal naming run-c, got: $got" >&2; exit 1 ;;
esac
