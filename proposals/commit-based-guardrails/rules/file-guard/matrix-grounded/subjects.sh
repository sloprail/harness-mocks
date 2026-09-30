#!/usr/bin/env bash
# One subject per matrix item this changeset added, changed or removed, with
# the statements of the invariants it covers (as committed at head).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/catalog.sh"
load_yaml spec/invariants.yaml; inv="$YAML"
changed_entries spec/matrix.yaml items | jq -c --argjson inv "${inv:-null}" '{subjects: map(
  . as $e
  | {id: .id, files: ["spec/matrix.yaml", "spec/invariants.yaml"],
     context: {change: .change, before: .before, after: .after,
       invariants: [ (($e.after // $e.before).covers // [])[] as $c
                     | (($inv // {}).invariants // [])[] | select(.id == $c) ]}})}'
