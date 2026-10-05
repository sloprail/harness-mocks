#!/usr/bin/env bash
# subjects: one per invariant touched: its spec file changed, or a changed file carries (before or
# after) sr:invariant <id> or sr:proves <id>.
#   files        the changed files that touch it: its spec file, and the files so marked
#   fingerprint  what the judge reads beyond them: the spec file (the statement) and every test
#                marked sr:proves <id>, changed or not.
# A change to invariant A leaves invariant B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_markers proves
# the markers go to jq through a file, not argv (Linux caps one argument at 128 KB); a failed jq
# refuses, never an empty list of subjects
arr="$(printf '%s' "$payload" | jq -c --rawfile proves <(printf '%s' "$MARKERS") '
  .changeset.files as $files
  | [$proves | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], q: .[1]}] as $pv
  | ([$files[] | (select(.path | test("^spec/invariants/[a-z0-9-]+\\.yaml$")) | .path | ltrimstr("spec/invariants/") | rtrimstr(".yaml")),
      (((.newMarkers // []) + (.oldMarkers // []))[] | select(.kind == "invariant" or .kind == "proves") | .fqn | select(test("/") | not))] | unique)
    | map(. as $id | {id: $id,
        files: [$files[] | select(.path == "spec/invariants/\($id).yaml"
                 or any(((.newMarkers // []) + (.oldMarkers // []))[]; (.kind == "invariant" or .kind == "proves") and .fqn == $id)) | .path],
        deps: (["spec/invariants/\($id).yaml"] + [$pv[] | select(.q == $id) | .p])})')" ||
  refuse "the touched invariants could not be worked out, so no subject could be made"
sub_finish unclaimed "$arr"
