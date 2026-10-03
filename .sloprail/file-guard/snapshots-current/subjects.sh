#!/usr/bin/env bash
# subjects: one per harness mock whose snapshots changed. A change to a capability file is every
# harness's: a cell cites that harness's docs and runs.
#   files        the changed files under <harness>-mock/snapshots/, and every changed
#                spec/capabilities file
#   fingerprint  what current.sh reads for the harness beyond them: its whole snapshots directory
#                (MANIFEST, capture.sh, every run and sample), the capability files (the cells that
#                cite them) and the list of harnesses.
# A change to claude-mock's snapshots leaves codex-mock's and cursor-mock's subjects as they were.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
arr="$(printf '%s' "$payload" | jq -c --arg hs "$(harnesses)" '
  [.changeset.files[].path] as $changed
  | [$changed[] | select(startswith("spec/capabilities/"))] as $caps
  | ([$hs | split("\n")[] | select(length > 0)] + [$changed[] | capture("^(?<h>[a-z0-9]+)-mock/snapshots/") | .h] | unique) as $all
  | [$all[] | . as $h | ([$changed[] | select(startswith("\($h)-mock/snapshots/"))] + $caps) as $files
     | select($files | length > 0)
     | {id: $h, files: $files, deps: ["\($h)-mock/snapshots", "spec/capabilities"], extra: ("harnesses:" + ($hs | gsub("\n"; " ")))}]')"
sub_finish unclaimed "$arr"
