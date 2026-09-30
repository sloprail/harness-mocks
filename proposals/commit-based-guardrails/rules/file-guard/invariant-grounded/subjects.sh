#!/usr/bin/env bash
# One subject per spec/invariants/<id>.yaml added, changed or removed.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
cs_json '{subjects: [.changeset.files[] | select(.path | test("^spec/invariants/[a-z0-9-]+\\.yaml$"))
  | {id: (.path | ltrimstr("spec/invariants/") | rtrimstr(".yaml")), files: [.path],
     context: {status, before: .oldContent, after: .newContent}}]}'
