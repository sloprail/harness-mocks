#!/usr/bin/env bash
# The judge's inputs exist, before a model is asked [deterministic]. For each
# touched (capability, harness): every cited doc is frozen in the MANIFEST (offline: the
# live page is never compared), every cited run has a sealed sample, and some test proves
# the pair. Each of these is another rule's to own (snapshots-current,
# capability-covered) across the whole tree; this re-checks only the slice the
# judge consumes, so a broken input refuses here, in one line naming its
# owner, instead of costing a judge call on an empty or missing <run>.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/pairs-lib.sh"
load_spec capabilities
load_markers proves; proves="$MARKERS"

problems=""
add() { problems="${problems}- $1"$'\n'; }
while IFS=$'\t' read -r pair cell _; do
  [ -n "$pair" ] || continue
  h="${pair#*/}"
  for ref in $(jq -r '.docs[]' <<<"$cell"); do
    doc_sha "$h" "$ref" >/dev/null || add "$pair: $h-mock/snapshots/MANIFEST.yaml does not freeze ${ref%%#*} (file-guard/snapshots-current owns this)"
  done
  for r in $(jq -r '.runs[]' <<<"$cell"); do
    ls "$SR_TREE/$r"/samples/*/SEAL >/dev/null 2>&1 ||
      add "$pair: $r has no sealed sample: capture it with $h-mock/snapshots/capture.sh run ${r##*/} (file-guard/snapshots-current owns this)"
  done
  printf '%s\n' "$proves" | awk -F'\t' -v q="$pair" '$2 == q && $1 ~ /_test\.go$/' | grep -q . ||
    add "$pair: no test carries // sr:proves $pair (file-guard/capability-covered owns this)"
done < <(rigor_pairs)

[ -z "$problems" ] && exit 0
refuse "Not judged yet: the judge's inputs are not ready. Fix these first, under the rule each names:
${problems}"
