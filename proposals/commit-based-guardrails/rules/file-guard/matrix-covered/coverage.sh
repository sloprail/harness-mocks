#!/usr/bin/env bash
# The coverage matrix, checked deterministically over the committed tree.
#
#   spec/invariants.yaml   invariants: [{id, statement}]
#   spec/matrix.yaml       items: [{id, covers: [invariant ids], case: {…}, expect}]
#   code                   // sr:invariant <invariant-id>     (non-test .go)
#   tests                  // sr:proves <matrix-item-id>      (*_test.go only)
#
# Every problem is collected, then all are reported at once: an agent fixing
# coverage should see the whole gap, not one cell per Stop.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"

load_yaml spec/invariants.yaml; inv="$YAML"
load_yaml spec/matrix.yaml; mat="$YAML"
[ "$inv" != "null" ] || refuse "spec/invariants.yaml is missing: the coverage matrix starts from the invariant catalog"
[ "$mat" != "null" ] || refuse "spec/matrix.yaml is missing: every invariant must be covered by at least one matrix item"

inv_ids="$(printf '%s' "$inv" | jq -r '(.invariants // [])[].id')"
item_ids="$(printf '%s' "$mat" | jq -r '(.items // [])[].id')"
load_markers invariant; impl="$MARKERS"
load_markers proves; proves="$MARKERS"

problems=""
add() { problems="${problems}- $1"$'\n'; }

for dup in $(printf '%s\n' "$inv_ids" | sort | uniq -d); do add "invariant id '$dup' is declared twice"; done
for dup in $(printf '%s\n' "$item_ids" | sort | uniq -d); do add "matrix item id '$dup' is declared twice"; done

# Each item covers known invariants and is proven by at least one test.
while IFS=$'\t' read -r id covers; do
  [ -n "$id" ] || continue
  [ -n "$covers" ] || add "matrix item '$id' covers no invariant"
  for c in $covers; do
    printf '%s\n' "$inv_ids" | grep -Fxq -- "$c" || add "matrix item '$id' covers unknown invariant '$c'"
  done
  printf '%s\n' "$proves" | awk -F'\t' -v id="$id" '$2 == id' | grep -q . ||
    add "matrix item '$id' has no test: add // sr:proves $id to a *_test.go that exercises it"
done < <(printf '%s' "$mat" | jq -r '(.items // [])[] | [.id, ((.covers // []) | join(" "))] | @tsv')

# Each invariant is covered by an item and implemented in non-test code.
while IFS= read -r id; do
  [ -n "$id" ] || continue
  printf '%s' "$mat" | jq -e --arg id "$id" 'any((.items // [])[]; (.covers // []) | index($id))' >/dev/null ||
    add "invariant '$id' is covered by no matrix item"
  printf '%s\n' "$impl" | awk -F'\t' -v id="$id" '$2 == id && $1 !~ /_test\.go$/' | grep -q . ||
    add "invariant '$id' has no implementation: mark the code that upholds it with // sr:invariant $id"
done <<<"$inv_ids"

# Markers name only declared ids, and proofs live in tests.
while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  printf '%s\n' "$item_ids" | grep -Fxq -- "$id" || add "$path: sr:proves '$id' names no matrix item"
  case "$path" in *_test.go) ;; *) add "$path: sr:proves belongs in a *_test.go, not in code" ;; esac
done <<<"$proves"
while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  printf '%s\n' "$inv_ids" | grep -Fxq -- "$id" || add "$path: sr:invariant '$id' names no invariant in spec/invariants.yaml"
done <<<"$impl"

[ -z "$problems" ] && exit 0
refuse "The coverage matrix has gaps:
${problems}Fix each one in a new commit. Removing an invariant or item, or changing what it says, needs the user's words (Sloprail-Cites-User)."
