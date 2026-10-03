#!/usr/bin/env bash
# touched_harnesses PATH: the harnesses whose providers cell in capability file
# PATH differs between the changeset's base and head, one per line; "*" when
# the file was added or removed, its statement changed, or the two versions
# cannot be read (then every providing harness is in question, the cautious
# side). A change to one harness's cell is judged on that cell alone, not on
# the other harnesses' cells it happens to share a file with. Source after
# changeset.sh.
touched_harnesses_slow() {
  local p="$1" base head b h
  base="$(cs '.changeset.base')"; head="$(cs '.changeset.head')"
  [ -n "$base" ] && [ -n "$head" ] || { echo '*'; return; }
  b="$(git -C "$SR_TREE" show "$base:$p" 2>/dev/null | yq -o=json '.' 2>/dev/null)" || b=""
  h="$(git -C "$SR_TREE" show "$head:$p" 2>/dev/null | yq -o=json '.' 2>/dev/null)" || h=""
  if [ -z "$b" ] || [ -z "$h" ]; then echo '*'; return; fi
  if [ "$(jq -cS '.statement' <<<"$b")" != "$(jq -cS '.statement' <<<"$h")" ]; then echo '*'; return; fi
  jq -rn --argjson b "$b" --argjson h "$h" \
    '(($b.providers // {}) + ($h.providers // {})) | keys[] as $k
     | select(($b.providers // {})[$k] != ($h.providers // {})[$k]) | $k' 2>/dev/null || echo '*'
}

# cell_kind CELL_JSON: what a providers cell is, one word.
#   supported    {docs, runs, deviations?}: mocked; covered, proven, judged
#   unsupported  {supported: false, reason, docs}: the harness lacks it, with evidence
#   pending      "pending": not mocked yet; tracked, never coverage
#   bare-false   false: absence without evidence; refused
#   invalid      anything else (shapes owns the details)
cell_kind() {
  jq -r 'if . == "pending" then "pending"
         elif . == false then "bare-false"
         elif type == "object" and .supported == false then "unsupported"
         elif type == "object" and (.supported == null) then "supported"
         else "invalid" end' <<<"$1"
}

# load_touched — sets TOUCHED_TSV to "<capability path><TAB><harness>" for every changed capability
# file, from one pass over the payload: both versions of every file come with it, and one yq
# parses them all. Call it in the shell that uses the table (a $(...) loses what it sets). When a
# file cannot be read that way, the table is built file by file (touched_harnesses_slow).
load_touched() {
  [ -z "${TOUCHED_READY:-}" ] || return 0
  TOUCHED_READY=1
  local paths p
  if TOUCHED_TSV="$(printf '%s' "$payload" | jq -c '[.changeset.files[] | select(.path | test("^spec/capabilities/[^/]+\\.yaml$"))
        | {path, st: .status, old: ((.oldContent // "") | if . == "" then "null" else . end), new: ((.newContent // "") | if . == "" then "null" else . end)}]' |
      yq -p=json -o=json -I=0 '.[] | .old |= (@yamld) | .new |= (@yamld)' 2>/dev/null |
      jq -r '. as $f | (if $f.st != "M" or ($f.old | type) != "object" or ($f.new | type) != "object" then ["*"]
             elif $f.old.statement != $f.new.statement then ["*"]
             else [(($f.old.providers // {}) + ($f.new.providers // {})) | keys[] as $k
                   | select(($f.old.providers // {})[$k] != ($f.new.providers // {})[$k]) | $k] end)
             | .[] | [$f.path, .] | join("\t")' 2>/dev/null)"; then
    return 0
  fi
  TOUCHED_TSV=""
  paths="$(cs '.changeset.files[].path | select(test("^spec/capabilities/[^/]+\\.yaml$"))')"
  for p in $paths; do
    TOUCHED_TSV="${TOUCHED_TSV}$(touched_harnesses_slow "$p" | awk -v p="$p" '{print p "\t" $0}')"$'\n'
  done
}

# touched_harnesses PATH — as touched_harnesses_slow, read from the table.
touched_harnesses() {
  load_touched
  printf '%s\n' "$TOUCHED_TSV" | awk -F'\t' -v p="$1" '$1 == p {print $2}'
}
