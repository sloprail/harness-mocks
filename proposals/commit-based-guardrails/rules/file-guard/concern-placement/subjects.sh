#!/usr/bin/env bash
# One subject per changed (added, modified, renamed) Go file.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
cs_json '{subjects: [.changeset.files[] | select(.status != "D")
  | {id: .path, files: [.path], context: {diff: .diff}}]}'
