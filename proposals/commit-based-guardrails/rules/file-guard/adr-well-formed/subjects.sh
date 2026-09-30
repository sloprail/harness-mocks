#!/usr/bin/env bash
# One subject per ADR.md added or changed.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
cs_json '{subjects: [.changeset.files[] | select(.status != "D" and (.path | test("^adr/[a-z0-9-]+/ADR\\.md$")))
  | {id: (.path | split("/")[1]), files: [.path], context: {text: .newContent, diff: .diff}}]}'
