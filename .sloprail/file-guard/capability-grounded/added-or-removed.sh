#!/usr/bin/env bash
# `when`: exit 0 (words required) when the changeset adds or deletes a
# capability file, or changes its statement (what is mocked is the user's call:
# adr/capability-grounding); exit 1 (waived) otherwise. A change confined to the
# support matrix (the `providers` cells: docs, runs, deviations) needs no words;
# the judge still reviews it. The statement is compared parsed, at the range's
# base and head, so an edit to any line of a folded block counts. Any failure
# requires them.
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
    yq -o=json '{"statement": .statement}' 2>/dev/null |
    jq -cS . 2>/dev/null || echo null
}
# the files this requirement is asked about: the subject's (the rule is split per capability, and
# another capability's statement moving must not demand words of this one), else every changed one
for p in $(cs '(.subject.files // [.changeset.files[].path])[] | select(test("^spec/capabilities/"))'); do
  b="$(decided "$base" "$p")"; h="$(decided "$head" "$p")"
  [ "$b" = "null" ] || [ "$h" = "null" ] && exit 0   # added or removed
  [ "$b" = "$h" ] || exit 0
done
exit 1
