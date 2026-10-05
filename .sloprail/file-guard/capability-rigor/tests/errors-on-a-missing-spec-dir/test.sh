#!/usr/bin/env bash
set -euo pipefail

# The scripts that read the capability specs through the shared load_spec (via pairs-lib.sh) refuse with
# "error": true when spec/capabilities is not in the tree they are handed: the tree failed, the change did not,
# so no verdict is cached. Never a pass, never a plain refusal, never a crash with no verdict.
# Each script is run directly on a committed tree that has no spec/ at all.
for p in /opt/homebrew/bin/yq /usr/local/bin/yq /usr/bin/yq; do [ -x "$p" ] && { mkdir -p bin && ln -sf "$p" bin/yq; break; }; done
export PATH="$PWD/bin:$PATH"
command -v yq >/dev/null || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p tree/claude-mock
export SR_TREE="$PWD/tree"
git -C tree init -q
payload='{"event":{"kind":"Changeset"},"changeset":{"files":[],"commits":[],"others":[],"citations":[]}}'

for s in capability-rigor/inputs-ready.sh capability-rigor/prepare.sh capability-rigor/subjects.sh capability-reconciled/reconciled.sh capability-reconciled/subjects.sh; do
  dir="$SR_TEST_SLOPRAIL_DIR/file-guard/$(dirname "$s")"
  rc=0
  out="$(cd "$dir" && printf '%s' "$payload" | SR_GUARDRAIL_DIR="$dir" SR_BASE=HEAD SR_HEAD=HEAD ./"$(basename "$s")" 2>/dev/null)" || rc=$?
  [ "$rc" -eq 1 ] || { echo "$s: exit $rc, wanted a refusal (1): $out" >&2; exit 1; }
  printf '%s' "$out" | jq -e '.error == true and (.reason | contains("spec/capabilities is not in the committed tree"))' >/dev/null ||
    { echo "$s: not an error verdict naming the missing spec dir: $out" >&2; exit 1; }
done
