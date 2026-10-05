#!/usr/bin/env bash
# subjects: one per spec/invariants/<id>.yaml added, changed or removed.
#   files        that file
#   fingerprint  its content at the range's base: the judge compares it with the head's ("before"),
#                and the base is not in the range's key.
# A change to invariant A leaves invariant B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
arr="$(cs_json '[.changeset.files[] | select(.path | test("^spec/invariants/[a-z0-9-]+\\.yaml$"))
  | {id: (.path | ltrimstr("spec/invariants/") | rtrimstr(".yaml")), files: [.path], bdeps: [.path]}]')" ||
  refuse_error "the changed invariant files could not be listed, so no subject could be made"
sub_finish unclaimed "$arr"
