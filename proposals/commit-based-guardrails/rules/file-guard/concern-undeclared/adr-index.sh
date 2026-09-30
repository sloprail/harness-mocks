#!/usr/bin/env bash
# prepare: the index of decided concerns: every ADR's and every module's
# one-line concern. A module's concern is decided too (its module.yaml). About 50 tokens an ADR,
# where the full texts would be several hundred each.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
load_adrs; load_modules
jq -n -c --argjson a "$ADRS" --argjson m "$MODULES" \
  '{additionalContext: {adrs: ([$a[] | {id: ("adr/" + .id), concern: (.frontmatter.concern // "")}] + [$m[] | {id: ("module " + .dir), concern}])}}'
