#!/usr/bin/env bash
# `when`: exit 0 (words required) when the changeset adds or deletes a
# capability file, changes its statement, or adds a claim about what the real
# harness does not do: a NEW deviation, or a cell that newly becomes
# `supported: false` or changes its `reason`. What is mocked, and what a mock
# or harness leaves out, is the user's call (adr/capability-grounding); exit 1
# (waived) otherwise. A change to a cell's docs and runs, or dropping a
# deviation, needs no words; the judge still reviews it. The one deviation that
# needs none is "Doc and recording conflict: ...", which records what the
# evidence shows and claims no absence. What the user decides is compared
# parsed, at the range's base and head, so an edit to any line of a folded
# block counts. Any failure requires the words.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
command -v jq >/dev/null 2>&1 && command -v yq >/dev/null 2>&1 || exit 0
base="$(cs '.changeset.base')"; head="$(cs '.changeset.head')"
[ -n "$base" ] && [ -n "$head" ] || exit 0
# what the user decides, of one capability file at REV ("null" if absent): the statement, the
# deviations per harness (a disclosed doc/recording conflict aside) and each unsupported cell's reason
decided() {   # REV PATH
  git -C "$SR_TREE" cat-file -e "$1:$2" 2>/dev/null || { echo null; return; }
  git -C "$SR_TREE" show "$1:$2" |
    yq -o=json '.' 2>/dev/null |
    jq -cS '{
      statement: .statement,
      deviations: [(.providers // {}) | to_entries[] | select(.value | type == "object") | .key as $h
        | (.value.deviations // [])[] | select((.statement // "") | test("^\\s*Doc and recording conflict:") | not)
        | {h: $h, adr: .adr, s: (.statement | gsub("\\s+"; " ") | sub("^ "; "") | sub(" $"; ""))}],
      absent: [(.providers // {}) | to_entries[] | select((.value | type == "object") and .value.supported == false)
        | {h: .key, r: (.value.reason // "")}]
    }' 2>/dev/null || echo null
}
# the files this requirement is asked about: the subject's (the rule is split per capability, and
# another capability's statement moving must not demand words of this one), else every changed one
for p in $(cs '(.subject.files // [.changeset.files[].path])[] | select(test("^spec/capabilities/"))'); do
  b="$(decided "$base" "$p")"; h="$(decided "$head" "$p")"
  [ "$b" = "null" ] || [ "$h" = "null" ] && exit 0   # added or removed
  jq -ne --argjson b "$b" --argjson h "$h" '
    $b.statement != $h.statement
    or (($h.deviations - $b.deviations) | length) > 0
    or (($h.absent - $b.absent) | length) > 0' >/dev/null && exit 0
done
exit 1
