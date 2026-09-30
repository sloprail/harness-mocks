#!/usr/bin/env bash
# Contract with the repo's replay tool (tools/replay, part of the harness-mocks
# core work): `go run ./tools/replay --harness <h> --run <snapshots/runs/<name>>`
# replays the run's setup against the mock, normalizes its events the way
# capture normalizes a sample's events.jsonl, and exits 0 when they equal one
# sample's, printing the first difference otherwise.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
load_spec capabilities; caps="$SPEC"
touched="$(cs '.changeset.files[].path')"
problems=""
for h in $(harnesses); do
  printf '%s\n' "$touched" | grep -Eq "^(core/|$h-mock/|spec/capabilities/)" || continue
  runs="$(jq -r --arg h "$h" '[.[] | .doc.providers[$h] // false | select(type == "object") | .runs[]] | unique | .[]' <<<"$caps")"
  [ -n "$runs" ] || continue
  [ -d "$SR_TREE/tools/replay" ] || refuse "capabilities cite $h runs, but tools/replay does not exist, so they cannot be replayed"
  for r in $runs; do
    out="$(cd "$SR_TREE" && go run ./tools/replay --harness "$h" --run "$h-mock/snapshots/runs/$r" 2>&1)" ||
      problems="${problems}- $h/$r: $(printf '%s' "$out" | head -c 600)"$'\n'
  done
done
[ -z "$problems" ] && exit 0
refuse "The mock no longer reproduces these recorded runs of the real harness:
${problems}"
