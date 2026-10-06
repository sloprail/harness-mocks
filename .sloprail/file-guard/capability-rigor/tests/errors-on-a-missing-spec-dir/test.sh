#!/usr/bin/env bash
set -euo pipefail

# capability-rigor (and the other scripts that read the capability specs through the shared load_spec,
# via pairs-lib.sh) when spec/capabilities is not in the tree they judge: the tree failed, the change did not,
# so the rule refuses with "error": true (no verdict, nothing cached; the engine words it "could not be
# evaluated") instead of a pass, a plain refusal or a crash.
#   1. through the engine: a recorded run changes while spec/capabilities is missing from head: capability-rigor
#      refuses as an error naming the missing spec dir
#   2. each script run directly on a tree with no spec/ at all: exit 1 and {"error": true, ...}
# the rules read the specs with yq, which the case's PATH (jq, git, bash, the sloprail binaries) does not carry
YQ="$(PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin" command -v yq)" || { echo "needs yq on the machine running the case" >&2; exit 1; }
mkdir -p "$TMPDIR/tools" && ln -sf "$YQ" "$TMPDIR/tools/yq" && export PATH="$PATH:$TMPDIR/tools"

git init -q .
c() { git add -A && git -c user.name=t -c user.email=t@t commit -q -m "$1"; }
mkdir -p claude-mock/snapshots/runs/r1 spec/capabilities
echo "version: 1" > claude-mock/snapshots/runs/r1/run.yaml
printf 'statement: x works\nproviders:\n  claude:\n    docs: [https://d.example/p#s]\n    runs: [claude-mock/snapshots/runs/r1]\n' > spec/capabilities/x.yaml
c base
BASE=$(git rev-parse HEAD)
git rm -q -r spec/capabilities
echo "version: 2" > claude-mock/snapshots/runs/r1/run.yaml; c "a run changes, spec/capabilities missing from the tree"
: > "$SR_EVENTS_FILE"
sr-checks run --base "$BASE" --head HEAD >/dev/null 2>&1 && ran=0 || ran=$?
[ "$ran" -ne 0 ]   # the range is refused
jq -es '[.[] | select(.kind=="FileGuardChecked" and .rule=="capability-rigor" and .on=="sr-checks run")] | length > 0 and all(.[]; .outcome=="refused" and (.reason | contains("could not be evaluated") and contains("spec/capabilities is not in the committed tree")))' "$SR_EVENTS_FILE" >/dev/null ||
  { jq -c 'select(.rule=="capability-rigor")' "$SR_EVENTS_FILE" >&2; echo "capability-rigor did not refuse as an error naming the missing spec dir" >&2; exit 1; }

# each script on its own, on a tree with no spec/
rm -rf tree && mkdir -p tree/claude-mock && git -C tree init -q
export SR_TREE="$PWD/tree"
payload='{"event":{"kind":"Changeset"},"changeset":{"files":[],"commits":[],"others":[],"citations":[]}}'
for s in capability-rigor/inputs-ready.sh capability-rigor/prepare.sh capability-rigor/subjects.sh capability-reconciled/reconciled.sh capability-reconciled/subjects.sh; do
  dir="$SR_TEST_SLOPRAIL_DIR/file-guard/$(dirname "$s")"
  rc=0
  out="$(cd "$dir" && printf '%s' "$payload" | SR_GUARDRAIL_DIR="$dir" SR_BASE=HEAD SR_HEAD=HEAD ./"$(basename "$s")" 2>/dev/null)" || rc=$?
  [ "$rc" -eq 1 ] || { echo "$s: exit $rc, wanted a refusal (1): $out" >&2; exit 1; }
  printf '%s' "$out" | jq -e '.error == true and (.reason | contains("spec/capabilities is not in the committed tree"))' >/dev/null ||
    { echo "$s: not an error verdict naming the missing spec dir: $out" >&2; exit 1; }
done

# a spec dir but no *-mock/: no harness cell can be checked, an error, not an empty pass
mkdir -p tree/spec/capabilities && printf 'statement: x works\nproviders:\n  claude: pending\n' > tree/spec/capabilities/x.yaml && rmdir tree/claude-mock
for s in capability-reconciled/reconciled.sh capability-reconciled/subjects.sh snapshots-current/current.sh; do
  dir="$SR_TEST_SLOPRAIL_DIR/file-guard/$(dirname "$s")"
  rc=0
  out="$(cd "$dir" && printf '%s' "$payload" | SR_GUARDRAIL_DIR="$dir" SR_BASE=HEAD SR_HEAD=HEAD ./"$(basename "$s")" 2>/dev/null)" || rc=$?
  [ "$rc" -eq 1 ] || { echo "$s: exit $rc, wanted a refusal (1): $out" >&2; exit 1; }
  printf '%s' "$out" | jq -e '.error == true and (.reason | contains("no *-mock/ directory"))' >/dev/null ||
    { echo "$s: not an error verdict naming the missing mock dirs: $out" >&2; exit 1; }
done
