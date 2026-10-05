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
# loaded here, in this shell: a $(...) loses what a loader sets, and a refusal inside one exits only it
load_spec capabilities; load_touched
load_touched_markers || refuse "the capability markers this change touches could not be worked out, so the judge's inputs could not be checked"

problems=""
add() { problems="${problems}- $1"$'\n'; }
# a failed lookup is a refusal, never an empty list that has nothing to check
pairs="$(rigor_pairs)" || refuse_error "the touched capability pairs could not be worked out, so the judge's inputs could not be checked"
while IFS=$'\t' read -r pair cell _; do
  [ -n "$pair" ] || continue
  h="${pair#*/}"
  docs="$(jq -r '(.docs // [])[]' <<<"$cell")" || refuse_error "$pair: its cited docs could not be listed, so they could not be checked"
  runs="$(jq -r '(.runs // [])[]' <<<"$cell")" || refuse_error "$pair: its cited runs could not be listed, so they could not be checked"
  for ref in $docs; do
    doc_sha "$h" "$ref" >/dev/null || add "$pair: $h-mock/snapshots/MANIFEST.yaml does not freeze ${ref%%#*} (file-guard/snapshots-current owns this)"
  done
  for r in $runs; do
    ls "$SR_TREE/$r"/samples/*/SEAL >/dev/null 2>&1 ||
      add "$pair: $r has no sealed sample: capture it with $h-mock/snapshots/capture.sh run ${r##*/} (file-guard/snapshots-current owns this)"
  done
  printf '%s\n' "$proves" | awk -F'\t' -v q="$pair" '$2 == q && $1 ~ /_test\.go$/ {ok = 1} END {exit !ok}' ||
    add "$pair: no test carries // sr:proves $pair (file-guard/capability-covered owns this)"
done <<<"$pairs"

[ -z "$problems" ] && exit 0
refuse "Not judged yet: the judge's inputs are not ready. Fix these first, under the rule each names:
${problems}"
