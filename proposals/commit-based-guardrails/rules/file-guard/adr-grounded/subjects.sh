#!/usr/bin/env bash
# One subject per ADR folder the changeset touched: its ADR.md before and
# after, and the diffs of every file in the folder that changed.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
cs_json '
  [.changeset.files[] | select(.path | test("^adr/[a-z0-9-]+/"))
   | . + {adr: (.path | split("/")[1])}]
  | group_by(.adr)
  | {subjects: map({
      id: .[0].adr,
      files: map(.path),
      context: {
        adr_before: ((map(select(.path | endswith("/ADR.md")))[0].oldContent) // ""),
        adr_after:  ((map(select(.path | endswith("/ADR.md")))[0].newContent) // ""),
        changes: map({path, status, diff})}})}'
