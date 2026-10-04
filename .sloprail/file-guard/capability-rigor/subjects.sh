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
slim_payload '^(spec/capabilities/|[a-z0-9]+-mock/snapshots/MANIFEST\.yaml$)'
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/snapshots.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
. "${SR_GUARDRAIL_DIR:-.}/pairs-lib.sh"
load_markers proves
# loaded here, in this shell: a $(...) loses what a loader sets
load_spec capabilities; load_touched; load_doc_changes; load_touched_markers; load_doc_shas
pairs="$(rigor_pairs)"   # one "<id>/<h>\t<cell>\t<capability>" per pair touched
arr="$(printf '%s' "$payload" | jq -c --arg pairs "$pairs" --arg proves "$MARKERS" --arg tm "$TOUCHED_MARKERS_TSV" --arg dc "$DOC_CHANGES_TSV" --argjson shas "$DOC_SHAS" '
  .changeset.files as $files
  | [$files[].path] as $changed
  | [$tm | split("\n")[] | select(length > 0) | split("\t") | {path: .[0], fqn: .[2]}] as $tmr
  | [$dc | split("\n")[] | select(length > 0) | split("\t") | {h: .[0], u: .[1]}] as $dcs
  | [$proves | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], q: .[1]}] as $pv
  | [$pairs | split("\n")[] | select(length > 0) | split("\t")
     | {pair: .[0], cell: (.[1] | fromjson), id: (.[0] | split("/")[0]), h: (.[0] | split("/")[1])}] as $pp
  | [$pp[].id] | unique | map(. as $id
      | [$pp[] | select(.id == $id)] as $mine
      | ([$id] + [$mine[].pair]) as $fq
      | {id: $id,
         files: ((if $changed | index("spec/capabilities/\($id).yaml") then ["spec/capabilities/\($id).yaml"] else [] end)
                 + ([$mine[] | select(.h as $h | any((.cell.docs // [])[]; (split("#")[0]) as $u | any($dcs[]; .h == $h and (.u == "*" or .u == $u)))) | "\(.h)-mock/snapshots/MANIFEST.yaml"] | unique)
                 + [$mine[].cell.runs[]? | . as $r | $changed[] | select(startswith($r + "/"))]
                 + [$tmr[] | select(.fqn as $f | $fq | index($f) != null) | .path]),
         deps: (["spec/capabilities/\($id).yaml"]
                + [$mine[] | (.cell.runs // [])[]]
                + [$mine[].pair as $pr | $pv[] | select(.q == $pr) | .p]),
         extra: ("pairs:" + ([$mine[].pair] | join(" "))
                 + "\ndocs:" + ([$mine[] | .h as $h | (.cell.docs // [])[] | (split("#")[0]) as $u | "\($h) \($u)=\($shas[$h].docs[$u] // "-")"] | unique | join(";")))})')"
sub_finish unclaimed "$arr"
