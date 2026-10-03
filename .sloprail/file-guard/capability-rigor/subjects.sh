#!/usr/bin/env bash
# subjects: one per capability with a (capability, harness) pair this changeset touches
# (pairs-lib.sh): the pairs of one capability are judged together, as one subject.
#   files        the changed files that touch it: its capability file, the files under the runs
#                it cites, the MANIFEST of each harness it provides, and the files whose
#                sr:capability / sr:provides / sr:proves markers name it (or one of its pairs)
#   fingerprint  what the judge reads beyond them: the pairs in question, the capability file,
#                and per pair its harness's MANIFEST (the docs' frozen hashes), each cited run
#                directory and every test marked sr:proves <id>/<harness>.
# A change to capability A leaves capability B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload '^spec/capabilities/'
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
. "${SR_GUARDRAIL_DIR:-.}/pairs-lib.sh"
load_markers proves
pairs="$(rigor_pairs)"   # one "<id>/<h>\t<cell>\t<capability>" per pair touched
arr="$(printf '%s' "$payload" | jq -c --arg pairs "$pairs" --arg proves "$MARKERS" '
  .changeset.files as $files
  | [$files[].path] as $changed
  | [$proves | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], q: .[1]}] as $pv
  | [$pairs | split("\n")[] | select(length > 0) | split("\t")
     | {pair: .[0], cell: (.[1] | fromjson), id: (.[0] | split("/")[0]), h: (.[0] | split("/")[1])}] as $pp
  | [$pp[].id] | unique | map(. as $id
      | [$pp[] | select(.id == $id)] as $mine
      | ([$id] + [$mine[].pair]) as $fq
      | {id: $id,
         files: ((if $changed | index("spec/capabilities/\($id).yaml") then ["spec/capabilities/\($id).yaml"] else [] end)
                 + [$mine[].h | select($changed | index("\(.)-mock/snapshots/MANIFEST.yaml")) | "\(.)-mock/snapshots/MANIFEST.yaml"]
                 + [$mine[].cell.runs[]? | . as $r | $changed[] | select(startswith($r + "/"))]
                 + [$files[] | select(any(((.newMarkers // []) + (.oldMarkers // []))[];
                       (.kind == "capability" or .kind == "provides" or .kind == "proves") and (.fqn as $f | $fq | index($f) != null))) | .path]),
         deps: (["spec/capabilities/\($id).yaml"]
                + [$mine[] | "\(.h)-mock/snapshots/MANIFEST.yaml", (.cell.runs // [])[]]
                + [$mine[].pair as $pr | $pv[] | select(.q == $pr) | .p]),
         extra: ("pairs:" + ([$mine[].pair] | join(" ")))})')"
sub_finish unclaimed "$arr"
