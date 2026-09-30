#!/usr/bin/env bash
# ADR-0004's deterministic half, over the whole committed tree:
#   - each sr:capability id is declared exactly once, under core/;
#   - sr:provides <id> <harness> sits under <harness>-mock/ and names a declared id.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
load_markers capability; caps="$MARKERS"
problems=""
while IFS= read -r id; do
  [ -n "$id" ] || continue
  sites="$(printf '%s\n' "$caps" | awk -F'\t' -v id="$id" '$2 == id {print $1}')"
  [ "$(printf '%s\n' "$sites" | grep -c .)" -eq 1 ] ||
    problems="${problems}- capability '$id' is implemented in more than one place: $(echo $sites)"$'\n'
done < <(printf '%s\n' "$caps" | awk -F'\t' 'NF {print $2}' | sort -u)
while IFS=$'\t' read -r path id; do
  [ -n "$path" ] || continue
  case "$path" in core/*) ;; *) problems="${problems}- $path declares capability '$id' outside core/"$'\n' ;; esac
done <<<"$caps"
# sr:provides carries two tokens; read them straight from the tree.
provides="$(git -C "$SR_TREE" grep -n -I -E '^[[:space:]]*(//|#|--)[[:space:]]*sr:provides[[:space:]]+' -- . ':!proposals/**' 2>&1)"
[ $? -le 1 ] || refuse "could not search the committed tree for sr:provides markers: $provides"
while IFS=: read -r path _ rest; do
  [ -n "$path" ] || continue
  set -- $(printf '%s' "$rest" | sed -E 's#^[[:space:]]*(//|\#|--)[[:space:]]*sr:provides[[:space:]]+##')
  id="${1:-}"; harness="${2:-}"
  case "$path" in "${harness}-mock/"*) ;; *) problems="${problems}- $path: sr:provides $id $harness must sit under ${harness}-mock/"$'\n' ;; esac
  printf '%s\n' "$caps" | awk -F'\t' -v id="$id" '$2 == id' | grep -q . ||
    problems="${problems}- $path provides '$id', which no core/ code declares with sr:capability"$'\n'
done <<<"$provides"
[ -z "$problems" ] && exit 0
refuse "ADR-0004 (each capability is implemented once, in core):
${problems}"
