#!/usr/bin/env bash
# subjects: one per ADR that changed, or that links a rule whose folder changed (what prepare.sh
# judges).
#   files        the changed files of the ADR (adr/<id>/...) and of each rule it links
#   fingerprint  what the judge reads beyond them: the ADR, each linked rule folder (every file of
#                it), the other ADRs that link those rules (a check one ADR does not decide may be
#                another's), and the shared _lib and schemas the rules source.
# A change to ADR A, or to a rule only A links, leaves ADR B's subject as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_adrs
arr="$(printf '%s' "$payload" | jq -c --argjson adrs "$ADRS" '
  [.changeset.files[].path] as $changed
  | [$adrs[] | . as $a | ($a.frontmatter.sloprails // []) as $links
     | {id: $a.id,
        files: ([$changed[] | select(startswith("adr/\($a.id)/"))] + [$links[] | . as $l | $changed[] | select(startswith(".sloprail/\($l)/"))]),
        deps: (["adr/\($a.id)/ADR.md", ".sloprail/_lib", ".sloprail/schemas"] + [$links[] | ".sloprail/\(.)"]
               + [$links[] | . as $l | $adrs[] | select(.id != $a.id and ((.frontmatter.sloprails // []) | index($l))) | .path])}
     | select(.files | length > 0)]')"
sub_finish unclaimed "$arr"
