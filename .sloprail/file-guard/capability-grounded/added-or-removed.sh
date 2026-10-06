#!/usr/bin/env bash
# `when`: exit 0 (words required) when the changeset adds or deletes a
# capability file, changes its statement, adds or changes a deviation whose `kind`
# is `mock-not-modeled` (or has no kind), or turns a cell `supported: false`
# without a cited recording; exit 1 (waived) otherwise. What is mocked, and what a
# mock leaves out, is the user's call (adr/capability-grounding, adr/modeled-surface).
# A deviation of kind `harness-lacks` (the harness itself differs from the statement:
# a recording or doc shows it), a cell that becomes `supported: false` with a recorded run,
# a change to a cell's docs and runs, and dropping a deviation need no words: the
# cited evidence grounds them, and the judge reviews it. A deviation that has no `kind`
# is not waived. A `mock-not-modeled` deviation is "unchanged" when the base has the same
# statement for that harness with that kind, or with no kind yet (the one-off migration that
# gave every deviation its kind). What the user decides is compared parsed, at the range's
# base and head, so an edit to any line of a folded block counts. Any failure requires the words:
# a lookup that fails applies the requirement (exit 0, with a hint saying so), it never waives it.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
command -v jq >/dev/null 2>&1 && command -v yq >/dev/null 2>&1 || exit 0
# applies WHY — the requirement applies, because what it depends on could not be worked out
applies() { jq -n --arg h "$1" '{hint: $h}'; exit 0; }
base="$(cs '.changeset.base')"; head="$(cs '.changeset.head')"
[ -n "$base" ] && [ -n "$head" ] || exit 0
# what the user decides, of one capability file at REV ("null" if absent): the statement, every
# deviation (harness, kind, normalized text) and the harnesses whose cell is `supported: false`
# with no recorded run
decided() {   # REV PATH
  git -C "$SR_TREE" cat-file -e "$1:$2" 2>/dev/null || { echo null; return; }
  git -C "$SR_TREE" show "$1:$2" |
    yq -o=json '.' 2>/dev/null |
    jq -cS '(.providers // {} | to_entries | map(select(.value | type == "object"))) as $cells | {
      statement: .statement,
      deviations: [$cells[] | .key as $h | (.value.deviations // [])[]
        | {h: $h, k: (.kind // null), s: (.statement | gsub("\\s+"; " ") | sub("^ "; "") | sub(" $"; ""))}],
      unrecorded: [$cells[] | select(.value.supported == false and ((.value.runs // []) | length) == 0) | .key]
    }' 2>/dev/null || echo null
}
# the files this requirement is asked about: the subject's (the rule is split per capability, and
# another capability's statement moving must not demand words of this one), else every changed one
files="$(cs '(.subject.files // [.changeset.files[].path])[] | select(test("^spec/capabilities/"))')" ||
  applies "the changed capability files could not be listed, so the user's words are required"
for p in $files; do
  b="$(decided "$base" "$p")"; h="$(decided "$head" "$p")"
  [ "$b" = "null" ] || [ "$h" = "null" ] && exit 0   # added or removed
  # jq -e: 0 the change needs words, 1 it does not, anything else (5: the program failed) is a failure
  jq -ne --argjson b "$b" --argjson h "$h" '
    $b.statement != $h.statement
    or ([$h.deviations[] | select(.k != "harness-lacks") | . as $d
          | select([$b.deviations[] | select(.h == $d.h and .s == $d.s
              and (.k == $d.k or (.k == null and $d.k == "mock-not-modeled")))] | length == 0)] | length) > 0
    or (($h.unrecorded - $b.unrecorded) | length) > 0' >/dev/null
  case $? in 0) exit 0 ;; 1) ;; *) applies "what $p changed could not be compared, so the user's words are required" ;; esac
done
exit 1
