#!/usr/bin/env bash
# One subject per invariant this changeset added, changed or removed.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/catalog.sh"
changed_entries spec/invariants.yaml invariants | jq -c '{subjects: map({
  id: .id, files: ["spec/invariants.yaml"],
  context: {change: .change, before: .before, after: .after}})}'
