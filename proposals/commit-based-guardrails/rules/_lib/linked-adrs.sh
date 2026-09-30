#!/usr/bin/env bash
# prepare for a rule's ADR judge: hands it the text of every ADR that links
# this rule (many-to-many), so the judge rules against each decision the rule
# enforces. A rule no ADR links has nothing to conform to: that is adr-linked's
# finding, and this judge is skipped rather than run against nothing.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
load_adrs "$(rule_qname)"
[ "$(jq 'length' <<<"$ADRS")" -gt 0 ] || { jq -n '{skip: true}'; exit 0; }
jq -n -c --argjson a "$ADRS" --arg rule "$(rule_qname)" '{additionalContext: {rule: $rule, adrs: $a}}'
