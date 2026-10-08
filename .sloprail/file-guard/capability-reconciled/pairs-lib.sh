#!/usr/bin/env bash
# reconcile_pairs — the (capability, harness) pairs this changeset touches, one "<id><TAB><harness>"
# per line (the harness is empty for a marker that names no harness). Touched:
#   - a capability file's cell changed (cells.sh: its statement, or the file being added or
#     removed, touches every harness: each mock, and each harness a marker names it for);
#   - a path under a run the cell cites changed (a recording deleted or replaced);
#   - a `sr:provides <id>/<h>` marker was added or removed (a file added, deleted or renamed:
#     every marker it has; a file modified: the markers its two sides do not share).
# The other side of a pair is read from the committed tree, never the changeset. Only the
# capability this check's subject names, when the rule is split. Source after changeset.sh,
# spec.sh and cells.sh, with $provides set (load_markers provides).
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/cells.sh"
reconcile_pairs() {
  load_touched   # $SPEC is loaded by the caller (load_spec at top level: a refusal inside $(...) would not end the check)
  printf '%s' "$payload" | jq -r --slurpfile caps0 <(printf '%s' "$SPEC") --arg tt "$TOUCHED_TSV" --arg head "$provides" --arg hs "$(harnesses)" --arg want "$(subject_id)" '
    ($hs | split("\n") | map(select(length > 0))) as $H
    | def idof: split("/")[0];
      def hof: (split("/") | if length > 1 then .[1:] | join("/") else "" end);
    [$head | split("\n")[] | select(length > 0) | split("\t") | .[1]] as $marked
    | ([.changeset.files[] | . as $f
        | if .status != "M" then [((.oldMarkers // []) + (.newMarkers // []))[] | select(.kind == "provides") | .fqn]
          else ([(.oldMarkers // [])[] | select(.kind == "provides") | .fqn]) as $o
               | ([(.newMarkers // [])[] | select(.kind == "provides") | .fqn]) as $n
               | (($o - $n) + ($n - $o)) end
        | .[]]) as $byMarker
    | ([$tt | split("\n")[] | select(length > 0) | split("\t")
        | (.[0] | ltrimstr("spec/capabilities/") | rtrimstr(".yaml")) as $id | .[1] as $h
        | if $h == "*" then ($H + [$marked[] | select(idof == $id) | hof] | unique[]) | "\($id)/\(.)" else "\($id)/\($h)" end]) as $byCell
    | [.changeset.files[].path] as $changed
    | ([$caps0[0][] | .id as $id | (.doc.providers // {} | if type == "object" then . else {} end) | to_entries[]
        | select(.value | type == "object" and .supported == null) | .key as $h
        | select(any((.value.runs // [])[]; . as $r | any($changed[]; startswith($r + "/")))) | "\($id)/\($h)"]) as $byRun
    | ($byMarker + $byCell + $byRun) | unique[]
    | select($want == "" or idof == $want)
    | [idof, hof] | @tsv'
}
