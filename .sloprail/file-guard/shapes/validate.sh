#!/usr/bin/env bash
# Validates every added or changed file that has a schema; reports every
# problem in every file at once (sr-file validate lists them all).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/shape.sh"
schemas="${SR_GUARDRAIL_DIR:-.}/../../schemas"
problems=""
for p in $(cs '.changeset.files[] | select(.status != "D") | .path'); do
  s="$(schema_for "$p")"; [ -n "$s" ] || continue
  set -- $s
  out="$(sr-file validate "$SR_TREE/$p" --schema "$schemas/$1" --path "$2" 2>&1)" ||
    problems="${problems}${out//$SR_TREE\//}"$'\n'
done
[ -z "$problems" ] && exit 0
refuse "Files that do not match their schema (.sloprail/schemas/):
${problems}"
