#!/usr/bin/env bash
# Shared setup of the statement-scope cases. The judge here is a mock: it proves the handoff (the rubric
# that holds the tests to the STATEMENT is in the prompt, the statement/doc/runs/tests reach the judge) and the
# verdict path; how the real model reads the rubric is for sr-eval. Its rule is the rubric's: each clause of
# the statement (the words after "proves ") must be in a test or in a recorded run's events (a green replay);
# what only the doc page says is never demanded.
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
mkdir -p bin
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq ; do [ -x "$p" ] && ln -sf "$p" bin/yq && break; done
# the doc page: its clause (marker) plus a behaviour outside any statement (DOCONLY)
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
# scope_case <statement> <test body> <recorded events line>
scope_case() {
  printf 'statement: %s\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/c]\n' "$1" >spec/capabilities/c.yaml
  d=claude-mock/snapshots/runs/c/samples/20240101-000000
  mkdir -p "$d"; printf 'version: 1\n' >claude-mock/snapshots/runs/c/run.yaml
  printf '%s\n' "$3" >"$d/events.jsonl"; (cd "$d" && shasum -a 256 ./events.jsonl >SEAL)
  printf '// sr:proves c/claude\n%s\n' "$2" >claude-mock/e2e/c_test.go
  git add -A && git commit -q -m base
  BASE=$(git rev-parse HEAD)
  export SR_CHECKS_JUDGE_MOCKS='{"file-guard/capability-rigor/tests-prove-as-documented":"'"$SR_TEST_CASE_DIR"'/judge.sh"}'
  # the recording is recorded again: the capability is judged
  printf '%s\n' "$3 " >"$d/events.jsonl"; (cd "$d" && shasum -a 256 ./events.jsonl >SEAL)
  git add -A && git commit -q -m "record c again"
  status=0; sr-checks run --base "$BASE" --head HEAD >out 2>&1 || status=$?
}
