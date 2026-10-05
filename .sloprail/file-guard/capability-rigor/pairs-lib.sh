#!/usr/bin/env bash
# rigor_pairs — the (capability, harness) pairs this changeset touches, one
# per line: "<id>/<harness>\t<cell json>\t<capability json>". Touched: the
# capability file changed in that harness's cell (cells.sh; its statement, or
# the whole file, changing touches every providing harness); a marker naming it changed
# (sr:capability → every harness; sr:provides/sr:proves <id>/<h> → that harness);
# or a recording it cites for <h> changed (a run). A doc page re-frozen is none of these: docs
# follow recordings and never re-judge alone. touched.sh narrows a marker to its declaration,
# so a change to one function leaves another's capability alone. Only the capability this check's subject names, when the rule is split (subjects.sh).
# Fails (non-zero) when the pairs cannot be worked out: an empty list is "nothing touched", a failure is not,
# so a caller captures `pairs="$(rigor_pairs)" || refuse ...` and never reads a failed lookup as an empty one.
# The catalog and the tables go to jq through files, not the command line (one argv entry is capped at 128 KB on Linux).
# Source after changeset.sh, spec.sh and snapshots.sh; no event logic.
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/touched.sh"
rigor_pairs() {
  load_touched; load_touched_markers   # $SPEC is loaded by the caller (load_spec at top level: a refusal inside $(...) would not end the check)
  # one jq over the payload, the specs and the touched table: no process per capability or pair
  printf '%s' "$payload" | jq -r --slurpfile caps0 <(printf '%s' "$SPEC") --rawfile tt <(printf '%s' "$TOUCHED_TSV") --rawfile tm <(printf '%s' "$TOUCHED_MARKERS_TSV") --arg want "$(subject_id)" '
    $caps0[0] as $caps
    | [.changeset.files[].path] as $changed
    | [$tm | split("\n")[] | select(length > 0) | split("\t") | .[2]] as $fq
    | [$tt | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], h: .[1]}] as $tr
    | $caps[] | select($want == "" or .id == $want) | . as $c | .id as $id | "spec/capabilities/\($id).yaml" as $cp
    | (.doc.providers // {}) | to_entries[] | select(.value | type == "object" and .supported == null) | .key as $h | .value as $cell
    | select(any($tr[]; .p == $cp and (.h == "*" or .h == $h))
             or any($fq[]; . == $id or . == "\($id)/\($h)")
             or any(($cell.runs // [])[]; . as $r | any($changed[]; startswith($r + "/"))))
    | ["\($id)/\($h)", ($cell | tojson), ($c | tojson)] | join("\t")'
}
