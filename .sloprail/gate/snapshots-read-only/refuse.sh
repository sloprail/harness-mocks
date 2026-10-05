#!/usr/bin/env bash
# The match already selected a protected path; every write to it is refused.
set -uo pipefail
payload="$(cat)"
path="$(printf '%s' "$payload" | jq -r '.event.path // ""')"
harness="${path%%-mock/*}"
jq -n --arg r "$path is a snapshot of the real harness: it is captured, not written. Run $harness-mock/snapshots/capture.sh (run <scenario> | all; a run also re-freezes the docs its cells cite) instead. To change a scenario, edit its setup/ and re-capture it." '{reason: $r}'
exit 1
