#!/usr/bin/env bash
# prepare: every ADR in the committed tree, so the judge can tell whether the
# concern it finds is covered.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
adrs="[]"
for f in "$(tree)"/.sloprail/file-guard/adr-*/ADR.md; do
  [ -f "$f" ] || continue
  id="$(basename "$(dirname "$f")")"
  adrs="$(jq -c --arg id "$id" --rawfile t "$f" '. + [{id: $id, text: $t}]' <<<"$adrs")"
done
jq -n -c --argjson a "$adrs" '{additionalContext: {adrs: $a}}'
