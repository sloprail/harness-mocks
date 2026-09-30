#!/usr/bin/env bash
# adr/capability-once's deterministic half, over the whole committed tree.
# Proves spec/capabilities.yaml and the code agree, in both directions:
#
#   catalog → code   every capability has exactly one // sr:capability <id>,
#                    under core/; every `supported` cell has a
#                    // sr:provides <id> <harness> under <harness>-mock/
#   code → catalog   every sr:capability and sr:provides names a catalogued
#                    capability, and a provides sits in a `supported` cell
#   the table        every harness mock (a top-level <h>-mock/ directory) has
#                    a cell for every capability; n/a carries a reason
#
# Code implementing a capability WITHOUT a marker cannot be seen by a marker
# check: concern-placement's judge catches capability behaviour outside core/.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"

load_yaml spec/capabilities.yaml; cat_json="$YAML"
[ "$cat_json" != "null" ] || refuse "spec/capabilities.yaml is missing: it is the list of capabilities the mocks model"
load_markers capability; caps="$MARKERS"
provides="$(git -C "$SR_TREE" grep -n -I -E '^[[:space:]]*(//|#|--)[[:space:]]*sr:provides[[:space:]]+' -- . ':!proposals/**' 2>&1)"
[ $? -le 1 ] || refuse "could not search the committed tree for sr:provides markers: $provides"
# "path<TAB>id<TAB>harness" per provides marker
[ -z "$provides" ] || provides="$(printf '%s\n' "$provides" | sed -E \
  's#^([^:]+):[0-9]+:[[:space:]]*(//|\#|--)[[:space:]]*sr:provides[[:space:]]+([^[:space:]]+)[[:space:]]+([^[:space:]]+).*$#\1\t\3\t\4#')"
harnesses="$(cd "$SR_TREE" && for d in *-mock; do [ -d "$d" ] && echo "${d%-mock}"; done)"

problems=""
add() { problems="${problems}- $1"$'\n'; }

ids="$(jq -r '(.capabilities // [])[].id' <<<"$cat_json")"
for dup in $(printf '%s\n' "$ids" | sort | uniq -d); do add "capability '$dup' is listed twice in spec/capabilities.yaml"; done

while IFS= read -r id; do
  [ -n "$id" ] || continue
  # catalog → core implementation
  sites="$(printf '%s\n' "$caps" | awk -F'\t' -v id="$id" '$2 == id {print $1}')"
  n="$(printf '%s\n' "$sites" | grep -c .)"
  if [ "$n" -eq 0 ]; then add "capability '$id' has no implementation: mark its code in core/ with // sr:capability $id"
  elif [ "$n" -gt 1 ]; then add "capability '$id' is implemented in more than one place: $(echo $sites)"; fi
  # the table: a cell per harness mock
  for h in $harnesses; do
    cell="$(jq -c --arg id "$id" --arg h "$h" '[(.capabilities // [])[] | select(.id == $id)][0].providers[$h] // null' <<<"$cat_json")"
    has="$(printf '%s\n' "$provides" | awk -F'\t' -v id="$id" -v h="$h" '$2 == id && $3 == h' | grep -c .)"
    case "$cell" in
      null) add "capability '$id' has no cell for harness '$h': say supported, or {n/a: <reason>}" ;;
      '"supported"') [ "$has" -gt 0 ] || add "capability '$id' is supported by '$h' but no ${h}-mock/ code carries // sr:provides $id $h" ;;
      *) if jq -e '(.["n/a"] // "") | length > 0' <<<"$cell" >/dev/null 2>&1; then
           [ "$has" -eq 0 ] || add "capability '$id' is n/a for '$h', yet ${h}-mock/ code provides it"
         else add "capability '$id' × '$h' must be supported or {n/a: <reason>}, not $cell"; fi ;;
    esac
  done
done <<<"$ids"

# code → catalog
while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  printf '%s\n' "$ids" | grep -Fxq -- "$id" || add "$path: sr:capability '$id' is not in spec/capabilities.yaml"
  case "$path" in core/*) ;; *) add "$path implements capability '$id' outside core/" ;; esac
done <<<"$caps"
while IFS=$'\t' read -r path id h; do
  [ -n "$path" ] || continue
  printf '%s\n' "$ids" | grep -Fxq -- "$id" || add "$path: sr:provides '$id' is not in spec/capabilities.yaml"
  case "$path" in "${h}-mock/"*) ;; *) add "$path: sr:provides $id $h must sit under ${h}-mock/" ;; esac
done <<<"$provides"

[ -z "$problems" ] && exit 0
refuse "adr/capability-once (each capability implemented once, in core; the catalog and the code agree):
${problems}"
