#!/usr/bin/env bash
# subjects: one per (capability, harness) pair this changeset touches (pairs-lib.sh), judged per harness:
# one harness's missing tests never refuse a change that touches only another harness. A change to one
# harness's cell, runs or markers makes that harness's subject only; a change to the shared statement or
# the whole capability file makes every providing harness's subject. The id of a subject is "<id>/<harness>".
#   files        the changed files that touch it: its capability file, the files under the runs
#                its cell cites, and the files whose sr:provides / sr:proves markers
#                name this pair
#   fingerprint  what the judge reads beyond them: the pair in question, the capability file,
#                its cell's cited run directories and every test marked sr:proves <id>/<harness>.
# No doc content is in the key: recordings own the truth, and a doc re-freeze alone re-judges
# nothing (the judge may still READ the docs, by path).
# A change to capability A leaves capability B's subjects, and to harness X leaves harness Y's, as they were.
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
load_touched_markers || refuse_error "the capability markers this change touches could not be worked out, so nothing could be judged"
# a failed lookup is a refusal, never an empty list (which would be the `unclaimed` subject, judging nothing)
pairs="$(rigor_pairs)" || refuse_error "the touched capability pairs could not be worked out, so nothing could be judged"   # one "<id>/<h>\t<cell>\t<capability>" per pair touched
# the pairs (each carries its cell and the whole capability) and the tables go through files, not the command line
arr="$(printf '%s' "$payload" | jq -c --rawfile pairs <(printf '%s' "$pairs") --rawfile proves <(printf '%s' "$MARKERS") --rawfile tm <(printf '%s' "$TOUCHED_MARKERS_TSV") '
  .changeset.files as $files
  | [$files[].path] as $changed
  | [$tm | split("\n")[] | select(length > 0) | split("\t") | {path: .[0], fqn: .[2]}] as $tmr
  | [$proves | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], q: .[1]}] as $pv
  | [$pairs | split("\n")[] | select(length > 0) | split("\t")
     | {pair: .[0], cell: (.[1] | fromjson), id: (.[0] | split("/")[0]), h: (.[0] | split("/")[1])}] as $pp
  | $pp | map(. as $p | $p.id as $id | [$p] as $mine
      | [$p.pair] as $fq
      | {id: $p.pair,
         files: ((if $changed | index("spec/capabilities/\($id).yaml") then ["spec/capabilities/\($id).yaml"] else [] end)
                 + [$mine[].cell.runs[]? | . as $r | $changed[] | select(startswith($r + "/"))]
                 + [$tmr[] | select(.fqn as $f | $fq | index($f) != null) | .path]),
         deps: (["spec/capabilities/\($id).yaml"]
                + [$mine[] | (.cell.runs // [])[]]
                + [$mine[].pair as $pr | $pv[] | select(.q == $pr) | .p]),
         extra: ("pair:" + $p.pair)})')" ||
  refuse_error "the capabilities this change touches could not be worked out, so nothing could be judged"
sub_finish unclaimed "$arr"
