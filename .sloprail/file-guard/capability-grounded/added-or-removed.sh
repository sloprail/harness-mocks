#!/usr/bin/env bash
# `when`: exit 0 (words required) when the changeset adds or deletes a
# capability file, or changes its statement or any cell's `deviations` (what is
# mocked, and where a mock knowingly differs, is the user's call:
# adr/capability-grounding); exit 1 (waived) otherwise. The statement and the
# deviations are compared parsed, at the range's base and head, so an edit to
# any line of a folded block counts. Any failure requires them.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
command -v jq >/dev/null 2>&1 && command -v yq >/dev/null 2>&1 || exit 0
base="$(cs '.changeset.base')"; head="$(cs '.changeset.head')"
[ -n "$base" ] && [ -n "$head" ] || exit 0
# what the user decides, of one capability file at REV ("null" if absent)
decided() {   # REV PATH
  git -C "$SR_TREE" cat-file -e "$1:$2" 2>/dev/null || { echo null; return; }
  git -C "$SR_TREE" show "$1:$2" |
    yq -o=json '{"statement": .statement, "deviations": ([.providers // {} | to_entries[] | {"h": .key, "d": (.value.deviations // [])}])}' 2>/dev/null |
    jq -cS . 2>/dev/null || echo null
}
for p in $(cs '.changeset.files[] | select(.path | test("^spec/capabilities/")) | .path'); do
  b="$(decided "$base" "$p")"; h="$(decided "$head" "$p")"
  [ "$b" = "null" ] || [ "$h" = "null" ] && exit 0   # added or removed
  [ "$b" = "$h" ] || exit 0
done
exit 1
