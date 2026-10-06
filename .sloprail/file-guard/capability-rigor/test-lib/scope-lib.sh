#!/usr/bin/env bash
# Shared setup of the statement-scope cases (each case reads it from $SR_TEST_SLOPRAIL_DIR: a case dir is
# copied alone). The judge here is a mock (judge.sh beside this file): it proves the handoff (the rubric that
# holds the tests to the STATEMENT is in the prompt, the statement/doc/runs/tests reach the judge, each run
# carries its replays flag) and the verdict path. How the real model reads the rubric is not covered here and
# has no sr-eval fixture yet; it needs one (run-eval skill).
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq ; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# the doc page: its clause plus a behaviour outside any statement (DOCONLY)
printf '#!/bin/sh\nout=""; while [ $# -gt 0 ]; do [ "$1" = -o ] && out="$2"; shift; done\nprintf "# s\\nthe harness does the thing. It also does DOCONLY.\\n" >"$out"\n' >bin/curl
chmod +x bin/curl
export PATH="$PWD/bin:$PATH"
SHA="$(printf '# s\nthe harness does the thing. It also does DOCONLY.\n' | shasum -a 256 | cut -d' ' -f1)"
git init -q .
printf 'bin\nout\n' >.gitignore
cp -R "$SR_TEST_SLOPRAIL_DIR" .sloprail
rm -rf .sloprail/tests .sloprail/file-guard/*/tests .sloprail/gate/*/tests .sloprail/file-guard/structure.tests
printf 'disabled:\n  - sloprail/gate/ci-verify-required\n  - sloprail/file-guard/rule-tests-pass\n' >.sloprail/config.yaml
mkdir -p spec/capabilities claude-mock/snapshots claude-mock/e2e
printf 'pin: "1"\ndocs:\n  https://d.example/p:\n    sha256: %s\n    fetched: "2026-10-01"\n' "$SHA" >claude-mock/snapshots/MANIFEST.yaml
# the mock's replay exception list (prepare.sh reads it): empty, or holding the run c with the given reason
replay_list() {
  mkdir -p claude-mock/e2e/018_replay
  { printf 'package e2e\n\nvar notReplaying = map[string]string{\n'
    [ -n "${1:-}" ] && printf '\t"c": "mock gap: %s",\n' "$1"
    printf '}\n'; } >claude-mock/e2e/018_replay/replay_allowlist_test.go
}
# scope_case <statement> <test body> <recorded events line> [reason: run c is on the replay exception list]
scope_case() {
  replay_list "${4:-}"
  printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml
  d=claude-mock/snapshots/runs/c/samples/20240101-000000
  mkdir -p "$d"; printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
  printf '%s\n' "$3" >"$d/events.jsonl"; (cd "$d" && shasum -a 256 ./events.jsonl >SEAL)
  printf '// sr:proves c/claude\n%s\n' "$2" >claude-mock/e2e/c_test.go
  git add -A && git commit -q -m base
  BASE=$(git rev-parse HEAD)
  export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-rigor/tests-prove-as-documented":"'"$SR_TEST_SLOPRAIL_DIR"'/file-guard/capability-rigor/test-lib/judge.sh"}'
  # the recording is recorded again: the capability is judged
  printf '%s\n' "$3 " >"$d/events.jsonl"; (cd "$d" && shasum -a 256 ./events.jsonl >SEAL)
  git add -A && git commit -q -m "record c again"
  status=0; sr-checks run --base "$BASE" --head HEAD >out 2>&1 || status=$?
}
# add_test <assertion line>: the fix a refusal asks for, a test asserting the clause, judged again
add_test() {
  printf '%s\n' "$1" >>claude-mock/e2e/c_test.go
  git add -A && git commit -q -m "a test for the clause"
  : >"$SR_EVENTS_FILE"
  status=0; sr-checks run --base "$BASE" --head HEAD >out 2>&1 || status=$?
}
