#!/usr/bin/env bash
set -euo pipefail
# A rule that cannot work out what to check refuses; it never passes on an empty lookup. With no *-mock/
# directory in the tree (an incomplete tree), snapshots-current has no harness to check: it refuses,
# and with a mock directory beside the same capability change it passes again.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
# the case's PATH is minimal: the rules' own tool, yq, is linked in from where the machine has it
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq ; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
export PATH="$PWD/bin:$PATH"
git init -q .
printf 'bin\nout\n' >.gitignore
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
git add -A && git commit -q -m base
BASE=$(git rev-parse HEAD)
mkdir -p spec/capabilities
printf 'statement: c works\nproviders: {}\n' >spec/capabilities/c.yaml
git add -A && git commit -q -m "a capability, and no mock directory"
run() { sr-checks run --base "$BASE" --head HEAD >out 2>&1 && ran=0 || ran=$?; }

: >"$SR_EVENTS_FILE"
run
jq -es 'any(.[]; .kind=="FileGuardChecked" and .rule=="snapshots-current" and .outcome=="refused" and (.reason | contains("no *-mock/ directory in the committed tree")))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "snapshots-current did not refuse a tree with no *-mock/ directory (sr-checks exit $ran)" >&2; exit 1; }

# recovery: a mock directory with no snapshots and a capability citing none, and the same range passes
mkdir -p claude-mock
printf 'package main\n' >claude-mock/main.go
git add -A && git commit -q -m "a mock directory"
: >"$SR_EVENTS_FILE"
run
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="snapshots-current")] | length > 0 and all(.[]; .outcome=="passed")' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c . "$SR_EVENTS_FILE" >&2; cat out >&2; echo "snapshots-current did not pass once a mock directory exists (sr-checks exit $ran)" >&2; exit 1; }
