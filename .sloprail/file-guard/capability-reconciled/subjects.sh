#!/usr/bin/env bash
# subjects: one per capability with a (capability, harness) pair this change touches (pairs-lib.sh).
#   files        the changed files that touch it: its capability file, the files under the runs its
#                touched cells cite, and those whose sr:provides markers name it
#   fingerprint  what the verdict reads beyond them: the pairs in question, the capability file,
#                the cited run directories, and every file at the head that carries a marker for
#                its pairs (the counterpart of a change to the cell, which the range may not select).
# A change to capability A leaves capability B's subject (files and fingerprint) as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
slim_payload '^spec/capabilities/'
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/spec.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
load_markers provides; provides="$MARKERS"
. "${SR_GUARDRAIL_DIR:-.}/pairs-lib.sh"
load_spec capabilities
pairs="$(reconcile_pairs)" || refuse "the touched capability pairs could not be worked out, so nothing could be reconciled"
arr="$(printf '%s' "$payload" | jq -c --arg pairs "$pairs" --arg head "$provides" --slurpfile caps0 <(printf '%s' "$SPEC") '
  [.changeset.files[].path] as $changed
  | [$pairs | split("\n")[] | select(length > 0) | split("\t") | {id: .[0], h: .[1]}] as $pp
  | [$head | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], f: .[1]}] as $hm
  | [$pp[].id] | unique | map(. as $id
      | [$pp[] | select(.id == $id) | .h] as $hs
      | ([$hs[] | "\($id)/\(.)"]) as $fq
      | ([$caps0[0][] | select(.id == $id) | (.doc.providers // {}) | (if type == "object" then . else {} end) | to_entries[]
          | select(.key as $k | $hs | index($k)) | (.value | if type == "object" then (.runs // []) else [] end)[]]) as $runs
      | {id: $id,
         files: ((if $changed | index("spec/capabilities/\($id).yaml") then ["spec/capabilities/\($id).yaml"] else [] end)
                 + [$runs[] | . as $r | $changed[] | select(startswith($r + "/"))]
                 + [$hm[] | select(.f as $f | $fq | index($f) != null) | .p | select(. as $p | $changed | index($p) != null)]),
         deps: (["spec/capabilities/\($id).yaml"] + $runs + [$hm[] | select(.f as $f | $fq | index($f) != null) | .p]),
         extra: ("pairs:" + ($hs | join(" ")))})')"
# a changed marker file that no longer carries the marker (deleted, or the marker removed) is still the subject's file
arr="$(printf '%s' "$payload" | jq -c --argjson arr "$arr" --arg pairs "$pairs" '
  [.changeset.files[] | . as $f | ((.oldMarkers // []) | map(select(.kind == "provides") | .fqn | split("/")[0]))[] | {id: ., p: $f.path}] as $old
  | $arr | map(. as $s | .files = ((.files + [$old[] | select(.id == $s.id) | .p]) | unique))')"
sub_finish unclaimed "$arr"
