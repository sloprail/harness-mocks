#!/usr/bin/env bash
set -euo pipefail

# The CI path, no agent turn: `sr-checks run` judges committed ranges with the project's rules; only
# this rule's outcome is asserted. The two timed replays (all-hooks-close-first and -second) may be skipped by a go
# test -skip in a workflow or a mock's Makefile when the same workflow runs them on their own three times; any
# other -skip of a replay (another pattern, a broader TIMED_REPLAYS, a skip with nowhere that runs them) is refused.
git init -q .
. "$SR_TEST_SLOPRAIL_DIR/file-guard/replay-exceptions-only-shrink/tests/_setup.sh"
install_checker || exit 1
mkdir -p .github/workflows claude-mock
wf=.github/workflows/test.yml
mk=claude-mock/Makefile
timed='TestGeneratedReplay/all-hooks-close-(first|second)'
workflow() {   # TIMED_REPLAYS-VALUE SKIP-VALUE [no-linux-run | no-pull-request | timed-on-macos]
  local trigger='  pull_request:' os=ubuntu-latest
  [ "${3:-}" = no-pull-request ] && trigger='  push:'
  [ "${3:-}" = timed-on-macos ] && os=macos-latest
  printf 'on:\n%s\nenv:\n  TIMED_REPLAYS: '"'"'%s'"'"'\njobs:\n  macos:\n    runs-on: macos-latest\n    steps:\n      - run: make -C claude-mock test-e2e E2E_TEST_FLAGS="-skip '"'"'%s'"'"'"\n' "$trigger" "$1" "$2" > "$wf"
  [ "${3:-}" = no-linux-run ] || printf '  linux:\n    runs-on: %s\n    steps:\n      - run: go test -race -count=3 -v ./claude-mock/e2e/018_replay -run '"'"'%s'"'"'\n' "$os" "$timed" >> "$wf"
}
printf 'E2E_TEST_FLAGS ?=\n' > "$mk"
printf 'jobs:\n  macos:\n    steps:\n      - run: make -C claude-mock test-e2e\n' > "$wf"
git add -A && git -c user.name=t -c user.email=t@t commit -q -m "ci without a skip"
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
commit() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }

# only the first timed replay is skipped: refused
git checkout -q -b first-only "$BASE"
workflow "$timed" 'TestGeneratedReplay/all-hooks-close-first'; commit "skip one replay"
refuses "a skip of one replay" "a go test -skip of 'TestGeneratedReplay/all-hooks-close-first'"

# every generated replay is skipped: refused
git checkout -q -b all "$BASE"
workflow "$timed" 'TestGeneratedReplay'; commit "skip every replay"
refuses "a skip of every replay" "a go test -skip of 'TestGeneratedReplay'"

# TIMED_REPLAYS is widened and the skip goes through it: refused
git checkout -q -b widened "$BASE"
workflow 'TestGeneratedReplay' '$TIMED_REPLAYS'; commit "widen TIMED_REPLAYS"
refuses "a widened TIMED_REPLAYS" "does not define TIMED_REPLAYS as exactly"

# the timed pair is skipped and nothing runs it: refused
git checkout -q -b nowhere "$BASE"
workflow "$timed" '$TIMED_REPLAYS' no-linux-run; commit "skip the timed replays with no run"
refuses "the timed replays skipped and run nowhere" "no step runs them on their own with -count=3"

# the timed pair is run on Linux but the workflow is not on every PR: refused
git checkout -q -b no-pr "$BASE"
workflow "$timed" '$TIMED_REPLAYS' no-pull-request; commit "the workflow is not triggered by pull requests"
refuses "the timed replays not run on every PR" "which must run on every PR"

# the timed pair is run three times but on macOS, whose timers cannot keep their gaps: refused
git checkout -q -b on-macos "$BASE"
workflow "$timed" '$TIMED_REPLAYS' timed-on-macos; commit "the timed replays run on macOS"
refuses "the timed replays run on macOS" "the job that runs them is not on Linux"

# a mock's Makefile skips another replay: refused
git checkout -q -b makefile "$BASE"
printf "E2E_TEST_FLAGS ?= -skip 'TestGeneratedReplay/bgagent'\n" > "$mk"; commit "skip a replay in the Makefile"
refuses "a Makefile skip of another replay" "claude-mock/Makefile: a go test -skip of 'TestGeneratedReplay/bgagent'"

# recovery: the timed pair skipped through TIMED_REPLAYS, run on its own three times, and the same range from the same base passes
git checkout -q -b timed "$BASE"
workflow "$timed" '$TIMED_REPLAYS'; commit "skip the timed replays on macOS, run them on Linux"
passes "the timed replays skipped on macOS and run on Linux"
