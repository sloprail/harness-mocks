#!/usr/bin/env bash
# prepare: the ADR index (id, one-line concern). About 50 tokens an ADR,
# where the full texts would be several hundred each.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_adrs
jq -n -c --argjson a "$ADRS" '{additionalContext: {adrs: [$a[] | {id, concern: (.frontmatter.concern // "")}]}}'
