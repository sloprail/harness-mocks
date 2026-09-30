#!/usr/bin/env bash
# prepare: every ADR in the committed tree (text + home), so the judge can
# tell whether a concern is decided, and where it must live.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_adrs
jq -n -c --argjson a "$ADRS" '{additionalContext: {adrs: [$a[] | {id, home: (.frontmatter.home // []), text}]}}'
