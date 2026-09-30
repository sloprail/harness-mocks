#!/usr/bin/env bash
# prepare for an ADR rule's judge: hands it its own ADR.md (from the rule's
# folder) and the diff of the files it matched, so the judge reads nothing else.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
adr="${SR_GUARDRAIL_DIR:-.}/ADR.md"
[ -f "$adr" ] || refuse "this ADR rule has no ADR.md beside it"
jq -n -c --rawfile adr "$adr" --arg name "${SR_GUARDRAIL:-adr}" '{additionalContext: {adr: $adr, name: $name}}'
