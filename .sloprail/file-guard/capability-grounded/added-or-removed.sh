#!/usr/bin/env bash
# `when`: exit 0 (words required) when the changeset adds or deletes a
# capability file, or changes anything of it outside the per-harness support
# matrix: its statement, or which harnesses provide it (a `providers.<h>` key
# added or dropped, or flipped between `false` and a cell). What is mocked is
# the user's call (adr/capability-grounding). Exit 1 (waived) when only the
# CONTENTS of a provider cell changed (docs, runs, deviations): the cells'
# quality is the judge's (docs-support-statement), not the user's words. Both
# sides are compared parsed, at the range's base and head, with every cell's
# contents masked, so an edit to any line of a folded block counts. Any
# failure requires the words.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
command -v jq >/dev/null 2>&1 && command -v yq >/dev/null 2>&1 || exit 0
base="$(cs '.changeset.base')"; head="$(cs '.changeset.head')"
[ -n "$base" ] && [ -n "$head" ] || exit 0
# what the user decides, of one capability file at REV ("null" if absent): the
# whole file with each provider cell's contents masked (a cell stays `false`
# or becomes "cell"), so only the matrix's shape is left beside the statement
decided() {   # REV PATH
  git -C "$SR_TREE" cat-file -e "$1:$2" 2>/dev/null || { echo null; return; }
  git -C "$SR_TREE" show "$1:$2" |
    yq -o=json '.' 2>/dev/null |
    jq -cS 'if (.providers | type) == "object" then .providers |= with_entries(.value |= (if . == false then false else "cell" end)) else . end' 2>/dev/null || echo null
}
for p in $(cs '.changeset.files[] | select(.path | test("^spec/capabilities/")) | .path'); do
  b="$(decided "$base" "$p")"; h="$(decided "$head" "$p")"
  [ "$b" = "null" ] || [ "$h" = "null" ] && exit 0   # added or removed
  [ "$b" = "$h" ] || exit 0
done
exit 1
