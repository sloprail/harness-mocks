#!/usr/bin/env bash
# The match already selected a protected path; every write to it is refused, and so is the deletion of
# anything under a run directory that is on main. A run directory that is not on main may be deleted
# (adr: a run not yet on main is the branch's own recording, only runs on main are read-only).
set -uo pipefail
payload="$(cat)"
path="$(printf '%s' "$payload" | jq -r '.event.path // ""')"
kind="$(printf '%s' "$payload" | jq -r '.event.kind // ""')"
harness="${path%%-mock/*}"
if [ "$kind" = PreFileDelete ]; then
  run="$(printf '%s\n' "$path" | sed -n 's#^\([a-z0-9]*-mock/snapshots/runs/[a-z0-9-]*\)/.*#\1#p')"
  if [ -n "$run" ]; then
    ws="${SR_WORKSPACE:-.}"
    main=""
    for ref in refs/remotes/origin/main refs/heads/main; do
      if git -C "$ws" rev-parse --verify -q "$ref^{commit}" >/dev/null 2>&1; then main="$ref"; break; fi
    done
    if [ -z "$main" ]; then
      jq -n --arg r "$path is under a run directory, and there is no main branch to tell whether $run is on main: a recording that may be on main is read-only, so it is not deleted. Fetch main, or run $harness-mock/snapshots/capture.sh." '{reason: $r}'
      exit 1
    fi
    # a run directory that is not on main may be deleted
    git -C "$ws" cat-file -e "$main:$run" 2>/dev/null || exit 0
  fi
fi
jq -n --arg r "$path is a snapshot of the real harness: it is captured, not written. Run $harness-mock/snapshots/capture.sh (run <scenario> | all; a run also re-freezes the docs its cells cite) instead. To change a scenario, edit its setup/ and re-capture it." '{reason: $r}'
exit 1
