#!/usr/bin/env bash
# rigor_pairs — the (capability, harness) pairs this changeset touches, one
# per line: "<id>/<harness>\t<cell json>\t<capability json>". Touched: the
# capability file changed in that harness's cell (cells.sh; its statement, or
# the whole file, changing touches every providing harness); a marker naming it changed
# (sr:capability → every harness; sr:provides/sr:proves <id>/<h> → that harness);
# or a snapshot it cites for <h> changed (a run, or the MANIFEST: a re-frozen
# doc). Only the capability this check's subject names, when the rule is split (subjects.sh).
# Source after changeset.sh, spec.sh and snapshots.sh; no event logic.
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
rigor_pairs() {
  load_spec capabilities; load_touched
  # one jq over the payload, the specs and the touched table: no process per capability or pair
  printf '%s' "$payload" | jq -r --argjson caps "$SPEC" --arg tt "$TOUCHED_TSV" --arg want "$(subject_id)" '
    [.changeset.files[].path] as $changed
    | [.changeset.files[] | ((.newMarkers // []) + (.oldMarkers // []))[]
       | select(.kind == "capability" or .kind == "provides" or .kind == "proves") | .fqn] as $fq
    | [$tt | split("\n")[] | select(length > 0) | split("\t") | {p: .[0], h: .[1]}] as $tr
    | $caps[] | select($want == "" or .id == $want) | . as $c | .id as $id | "spec/capabilities/\($id).yaml" as $cp
    | (.doc.providers // {}) | to_entries[] | select(.value | type == "object" and .supported == null) | .key as $h | .value as $cell
    | select(any($tr[]; .p == $cp and (.h == "*" or .h == $h))
             or any($fq[]; . == $id or . == "\($id)/\($h)")
             or any(($cell.runs // [])[]; . as $r | any($changed[]; startswith($r + "/")))
             or any($changed[]; . == "\($h)-mock/snapshots/MANIFEST.yaml"))
    | ["\($id)/\($h)", ($cell | tojson), ($c | tojson)] | join("\t")'
}
