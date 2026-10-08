#!/usr/bin/env bash
# subjects: one per ADR folder the changeset touched.
#   files        its changed files (adr/<id>/...)
#   fingerprint  each file's content at the range's base: the judge reads it as "before", and the
#                base is not in the range's key.
# A change to ADR A leaves ADR B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
arr="$(cs_json '[.changeset.files[].path | select(test("^adr/[^/]+/")) | {id: split("/")[1], p: .}]
  | group_by(.id) | map({id: .[0].id, files: map(.p), bdeps: map(.p)})')" ||
  refuse_error "the touched ADRs could not be worked out, so no subject could be made"
sub_finish unclaimed "$arr"
