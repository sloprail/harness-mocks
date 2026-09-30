#!/usr/bin/env bash
# Which schema a path answers to. Source it; do not run it.
#
# schema_for PATH — prints "<schema file> <definition>", or nothing for a path
# no schema covers. The schema files live in .sloprail/schemas/.
schema_for() {
  case "$1" in
    spec/invariants/*.yaml) echo "invariant.cue #Invariant" ;;
    spec/capabilities/*.yaml) echo "capability.cue #Capability" ;;
    adr/*/ADR.md) echo "adr.cue #ADR" ;;
    *-mock/snapshots/MANIFEST.yaml) echo "manifest.cue #Manifest" ;;
    *-mock/snapshots/runs/*/run.yaml) echo "run.cue #Run" ;;
    */module.yaml) echo "module.cue #Module" ;;
  esac
}
