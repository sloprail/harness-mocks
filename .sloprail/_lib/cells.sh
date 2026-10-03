#!/usr/bin/env bash
# touched_harnesses PATH: the harnesses whose providers cell in capability file
# PATH differs between the changeset's base and head, one per line; "*" when
# the file was added or removed, its statement changed, or the two versions
# cannot be read (then every providing harness is in question, the cautious
# side). A change to one harness's cell is judged on that cell alone, not on
# the other harnesses' cells it happens to share a file with. Source after
# changeset.sh.
touched_harnesses() {
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

