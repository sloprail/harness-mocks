#!/usr/bin/env bash
# Invariants touched: a changed spec/invariants/<id>.yaml, or a changed file
# carrying (before or after) sr:invariant <id> or sr:proves <id>. Each subject
# carries the statement and the full text of every test file proving it.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
load_spec invariants; inv="$SPEC"
load_markers proves; proves="$MARKERS"
ids="$( { cs '.changeset.files[].path | select(test("^spec/invariants/[a-z0-9-]+\\.yaml$")) | ltrimstr("spec/invariants/") | rtrimstr(".yaml")'
          cs '.changeset.files[] | ((.newMarkers // []) + (.oldMarkers // []))[] | select(.kind == "invariant" or .kind == "proves") | .fqn | select(test("/") | not)'
        } | sort -u)"
subjects="[]"
while IFS= read -r id; do
  [ -n "$id" ] || continue
  st="$(jq -r --arg id "$id" '[.[] | select(.id == $id)][0].doc.statement // empty' <<<"$inv")"
  [ -n "$st" ] || continue                     # removed, or malformed: invariant-covered's finding
  tests="[]"
  for f in $(printf '%s\n' "$proves" | awk -F'\t' -v id="$id" '$2 == id {print $1}' | sort -u); do
    tests="$(jq -c --arg p "$f" --rawfile t "$SR_TREE/$f" '. + [{path: $p, text: $t}]' <<<"$tests")"
  done
  subjects="$(jq -c --arg id "$id" --arg st "$st" --argjson t "$tests" \
    '. + [{id: $id, files: (["spec/invariants/\($id).yaml"] + [$t[].path]), context: {statement: $st, tests: $t}}]' <<<"$subjects")"
done <<<"$ids"
jq -n -c --argjson s "$subjects" '{subjects: $s}'
