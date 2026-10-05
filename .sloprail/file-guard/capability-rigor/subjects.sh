#!/usr/bin/env bash
# subjects: one per capability with a (capability, harness) pair this changeset touches
# (pairs-lib.sh): the pairs of one capability are judged together, as one subject.
#   files        the changed files that touch it: its capability file, the files under the runs
#                it cites, and the files whose sr:capability / sr:provides / sr:proves markers
#                name it (or one of its pairs)
#   fingerprint  what the judge reads beyond them: the pairs in question, the capability file,
#                each cited run directory and every test marked sr:proves <id>/<harness>.
# No doc content is in the key: recordings own the truth, and a doc re-freeze alone re-judges
# nothing (the judge may still READ the docs, by path).
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
# loaded here, in this shell: a $(...) loses what a loader sets
load_spec capabilities; load_touched
load_touched_markers || refuse "the capability markers this change touches could not be worked out, so nothing could be judged"
# a failed lookup is a refusal, never an empty list (which would be the `unclaimed` subject, judging nothing)
pairs="$(rigor_pairs)" || refuse "the touched capability pairs could not be worked out, so nothing could be judged"   # one "<id>/<h>\t<cell>\t<capability>" per pair touched
# the pairs (each carries its cell and the whole capability) and the tables go through files, not the command line
arr="$(printf '%s' "$payload" | jq -c --rawfile pairs <(printf '%s' "$pairs") --rawfile proves <(printf '%s' "$MARKERS") --rawfile tm <(printf '%s' "$TOUCHED_MARKERS_TSV") '
  .changeset.files as $files
  | [$files[].path] as $changed
  | [$tm | split("\n")[] | select(length > 0) | split("\t") | {path: .[0], fqn: .[2]}] as $tmr
  | [$proves | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], q: .[1]}] as $pv
  | [$pairs | split("\n")[] | select(length > 0) | split("\t")
     | {pair: .[0], cell: (.[1] | fromjson), id: (.[0] | split("/")[0]), h: (.[0] | split("/")[1])}] as $pp
  | [$pp[].id] | unique | map(. as $id
      | [$pp[] | select(.id == $id)] as $mine
      | ([$id] + [$mine[].pair]) as $fq
      | {id: $id,
         files: ((if $changed | index("spec/capabilities/\($id).yaml") then ["spec/capabilities/\($id).yaml"] else [] end)
                 + [$mine[].cell.runs[]? | . as $r | $changed[] | select(startswith($r + "/"))]
                 + [$tmr[] | select(.fqn as $f | $fq | index($f) != null) | .path]),
         deps: (["spec/capabilities/\($id).yaml"]
                + [$mine[] | (.cell.runs // [])[]]
                + [$mine[].pair as $pr | $pv[] | select(.q == $pr) | .p]),
         extra: ("pairs:" + ([$mine[].pair] | join(" ")))})')" ||
  refuse "the capabilities this change touches could not be worked out, so nothing could be judged"
sub_finish unclaimed "$arr"
