#!/usr/bin/env bash
# Cells and adapters, both ways, for the pairs this change touches (pairs-lib.sh):
#   - each supported cell ({docs, runs}): ≥1 `// sr:provides <id>/<h>` under <h>-mock/, and ≥1 cited
#     recorded run, each an existing directory;
#   - each `// sr:provides <id>/<h>` marker (the touched pair's): <id> is a capability, <h>'s cell is
#     supported, and the code sits under <h>-mock/. A pending, unsupported or missing cell is no
#     coverage: no marker may name it.
# Removing the code, or making the cell unsupported, fails until the other side matches.
# The file's shape and the rest of the coverage (cells for every harness, the one sr:capability,
# sr:proves) are shapes' and capability-covered's.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload '^spec/capabilities/'
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
load_markers provides; provides="$MARKERS"
. "${SR_GUARDRAIL_DIR:-.}/pairs-lib.sh"
load_spec capabilities; caps="$SPEC"

pairs="$(reconcile_pairs)" || refuse "the touched capability pairs could not be worked out, so nothing could be reconciled"
problems=""
add() { problems="${problems}- $1"$'\n'; }
# `// "missing"` would read a false cell as missing: jq's // treats false as absent.
cell() { jq -c --arg id "$1" --arg h "$2" '[.[] | select(.id == $id)][0].doc.providers | if type == "object" and has($h) then .[$h] else "missing" end' <<<"$caps"; }
state() {   # CELL_JSON — what the cell is, in words
  case "$1" in
    '"missing"') echo "missing (the capability has no cell for it)" ;;
    '"pending"') echo "pending (not mocked yet)" ;;
    false) echo "a bare false" ;;
    *) case "$(cell_kind "$1")" in unsupported) echo "unsupported (supported: false)" ;; *) echo "not a supported cell" ;; esac ;;
  esac
}

while IFS=$'\t' read -r id h; do
  [ -n "$id" ] || continue
  if [ -z "$h" ]; then
    for p in $(printf '%s\n' "$provides" | awk -F'\t' -v id="$id" '$2 == id {print $1}'); do
      add "$p: sr:provides $id must be <capability>/<harness>"
    done
    continue
  fi
  fq="$id/$h"
  marks="$(printf '%s\n' "$provides" | awk -F'\t' -v f="$fq" '$2 == f {print $1}')"
  if ! jq -e --arg id "$id" 'any(.[]; .id == $id)' <<<"$caps" >/dev/null; then
    for p in $marks; do add "$p: sr:provides $fq names no capability ($id has no spec/capabilities/$id.yaml): remove the marker, or add the capability"; done
    continue
  fi
  v="$(cell "$id" "$h")"
  if [ "$(cell_kind "$v")" = supported ]; then
    [ -n "$marks" ] ||
      add "spec/capabilities/$id.yaml: '$id' × '$h' is a supported cell but no $h-mock/ code carries // sr:provides $fq: restore the adapter, or make the cell unsupported (with the user's words or a recording that shows the harness lacks it) or pending"
    for p in $marks; do
      case "$p" in "$h"-mock/*) ;; *) add "$p: sr:provides $fq must sit under $h-mock/" ;; esac
    done
    n="$(jq -r '(.runs // []) | length' <<<"$v")"
    [ "$n" -gt 0 ] 2>/dev/null || add "spec/capabilities/$id.yaml: '$id' × '$h' is a supported cell that cites no recorded run: cite at least one under $h-mock/snapshots/runs/"
    for r in $(jq -r '(.runs // [])[]' <<<"$v"); do
      [ -d "$SR_TREE/$r" ] || add "spec/capabilities/$id.yaml: '$id' × '$h' cites the run $r, which is not a directory in the tree"
    done
  else
    for p in $marks; do
      add "$p: sr:provides $fq, but the cell '$id' × '$h' is $(state "$v"): code carrying sr:provides is the adapter of a supported cell, so make the cell supported ({docs, runs}) or remove the marker"
    done
  fi
done <<<"$pairs"

[ -z "$problems" ] && exit 0
refuse "Cells and adapters do not match, so one side must follow the other (adr/capability-once):
${problems}"
