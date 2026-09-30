#!/usr/bin/env bash
# One subject per matrix item whose entry changed, or one of whose proving
# tests changed. Its context carries the item and the full text of EVERY test
# proving it (changed or not), read from the committed tree.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/catalog.sh"

load_yaml spec/matrix.yaml; mat="$YAML"
load_markers proves; proves="$MARKERS"

# Items touched: entries changed in the matrix, plus items named by a proves
# marker in a changed file (before or after the change).
ids="$( { changed_entries spec/matrix.yaml items | jq -r '.[] | select(.change != "removed") | .id'
          cs '.changeset.files[] | ((.newMarkers // []) + (.oldMarkers // []))[] | select(.kind == "proves") | .fqn'
        } | sort -u)"

subjects="[]"
while IFS= read -r id; do
  [ -n "$id" ] || continue
  item="$(printf '%s' "$mat" | jq -c --arg id "$id" '[((. // {}).items // [])[] | select(.id == $id)][0] // null')"
  [ "$item" != "null" ] || continue            # an unknown id is matrix-covered's finding
  tests="[]"
  for f in $(printf '%s\n' "$proves" | awk -F'\t' -v id="$id" '$2 == id {print $1}' | sort -u); do
    tests="$(jq -c --arg p "$f" --rawfile t "$(tree)/$f" '. + [{path: $p, text: $t}]' <<<"$tests")"
  done
  subjects="$(jq -c --arg id "$id" --argjson item "$item" --argjson tests "$tests" \
    '. + [{id: $id, files: ([$tests[].path] + ["spec/matrix.yaml"]), context: {item: $item, tests: $tests}}]' <<<"$subjects")"
done <<<"$ids"
jq -n -c --argjson s "$subjects" '{subjects: $s}'
