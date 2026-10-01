#!/usr/bin/env bash
# Validates the bytes a Write/Edit is about to leave. When sloprail cannot
# predict them (a shell edit), it defers to file-guard/shapes at the commit.
set -uo pipefail
payload="$(cat)"
refuse() { jq -n --arg r "$1" '{reason: $r}'; exit 1; }
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/shape.sh"
path="$(printf '%s' "$payload" | jq -r '.event.path // ""')"
[ "$(printf '%s' "$payload" | jq -r '.event.resultKnown')" = "true" ] || exit 0
s="$(schema_for "$path")"; [ -n "$s" ] || exit 0
set -- $s
case "$path" in *.md) as=.md ;; *) as=.yaml ;; esac
out="$(printf '%s' "$payload" | jq -r '.event.newContent // ""' |
  sr-file validate - --as "$as" --schema "${SR_GUARDRAIL_DIR:-.}/../../schemas/$1" --path "$2" 2>&1)" ||
  refuse "$path would not match its schema (.sloprail/schemas/$1):
$out"
exit 0
