#!/usr/bin/env bash
# Helpers for a rule's `subjects:` script. Source after changeset.sh (and the other libs it needs).
#
# `subjects:` splits a rule's changeset into units, each judged and stored on its own key. The
# key of a subject is its files' content plus its `fingerprint`, and nothing else, so:
#
#   - `files` are changed files the rule SELECTED (the engine refuses any other): the part of the
#     change that is this unit's;
#   - `fingerprint` must cover EVERYTHING ELSE the unit's verdict reads: a file the check opens
#     from $SR_TREE that did not change, a base-side content, a list of siblings. A dependency
#     left out is a stale verdict served after it changes: worse than no split.
#
# The script runs in `run` and in `verify` alike, with no session, from the head tree.
# The rule's own checks then read `.subject.id` (subject_id / want_subject in changeset.sh) and
# judge that unit alone: the payload they get is still the whole changeset.
#
#   payload="$(cat)"; . changeset.sh; . subjects.sh
#   sub_finish FALLBACK_ID "$(jq ... one array of {id, files, deps, bdeps, extra})"

SUBJECTS="[]"

# These scripts run in `verify` (Stop, pre-push, CI) as well as `run`, so they are written for
# speed: work from the changeset payload, never walk history, and spawn a fixed number of
# processes whatever the number of units (one jq over the whole payload, one `git cat-file
# --batch-check` for every object id, one `git hash-object` for every fingerprint).
#
# A script builds ONE JSON array of {id, files, deps, bdeps, extra} and hands it to sub_finish:
#   deps   paths whose content at the range's head the verdict depends on (a file or a directory)
#   bdeps  paths whose content at the range's base it depends on
#   extra  any other text the verdict depends on
# The fingerprint is a hash of the extra text and each dep's object id.

# The helpers below never call `refuse` inside a $(...) (it would exit only that subshell, and its
# JSON would land in the caller's variable): they set a global (OIDS, RESOLVED) in the caller's
# shell, and every lookup refuses on failure. Large values reach jq through files, never argv
# (Linux caps one argument at 128 KB).

# sub_oids REV PATHS — PATHS: one path per line; sets OIDS to the object id of each at REV ("-" if absent).
sub_oids() {
  local in out
  in="$(printf '%s\n' "$2" | sed "s|^|$1:|")" || refuse "could not name the paths to look up at $1"
  out="$(git -C "$SR_TREE" cat-file --batch-check <<<"$in" 2>&1)" || refuse "could not read object ids at $1: $out"
  [ "$(grep -c . <<<"$in")" -eq "$(grep -c . <<<"$out")" ] || refuse "object id lookup at $1 returned the wrong number of lines"
  OIDS="$(awk '{ if ($NF == "missing") print "-"; else print $1 }' <<<"$out")" || refuse "could not read the object ids at $1"
}

# sub_resolve ARRAY — sets RESOLVED to ARRAY with each subject's deps and bdeps resolved: [{id, files, text}]
sub_resolve() {
  local arr="$1" hp bp hm bm rev
  hp="$(jq -r '[.[].deps // [] | .[]] | unique | .[]' <<<"$arr")" || refuse "the subjects' deps could not be listed, so no subject could be made"
  bp="$(jq -r '[.[].bdeps // [] | .[]] | unique | .[]' <<<"$arr")" || refuse "the subjects' bdeps could not be listed, so no subject could be made"
  hm='{}'; bm='{}'
  if [ -n "$hp" ]; then
    rev="$(cs '.changeset.head')" || refuse "the range's head could not be read, so no subject could be made"
    sub_oids "$rev" "$hp"
    hm="$(paste <(printf '%s\n' "$hp") <(printf '%s\n' "$OIDS") | jq -Rn '[inputs | split("\t") | {(.[0]): .[1]}] | add')" ||
      refuse "the deps' object ids could not be read, so no subject could be made"
  fi
  if [ -n "$bp" ]; then
    rev="$(cs '.changeset.base')" || refuse "the range's base could not be read, so no subject could be made"
    sub_oids "$rev" "$bp"
    bm="$(paste <(printf '%s\n' "$bp") <(printf '%s\n' "$OIDS") | jq -Rn '[inputs | split("\t") | {(.[0]): .[1]}] | add')" ||
      refuse "the bdeps' object ids could not be read, so no subject could be made"
  fi
  RESOLVED="$(jq -c --slurpfile h <(printf '%s' "$hm") --slurpfile b <(printf '%s' "$bm") '[.[] | {id, files: (.files | unique), text: ((.extra // "")
      + ((.deps // []) | map("\n" + . + "=" + $h[0][.]) | join(""))
      + ((.bdeps // []) | map("\nbase:" + . + "=" + $b[0][.]) | join("")))}]' <<<"$arr")" ||
    refuse "the subjects' deps could not be resolved, so no subject could be made"
}

# sub_finish FALLBACK_ID ARRAY — prints the subjects of ARRAY (see above). The engine refuses an
# empty list for a rule that selected files, so when nothing claimed any (a file the rule selects
# but no unit owns) the whole selection becomes one subject named FALLBACK_ID, which the rule's
# checks find nothing to judge in.
sub_finish() {
  local arr n tmp i=0 line lines paths hashes out
  sub_resolve "$2"; arr="$RESOLVED"
  n="$(jq 'length' <<<"$arr")" || refuse "the subjects could not be counted, so no subject could be made"
  if [ "$n" -eq 0 ]; then
    cs_json '[{id: "'"$1"'", files: ([.changeset.files[].path] | unique)}]' || refuse "the unclaimed files could not be listed, so no subject could be made"
    return 0
  fi
  tmp="$(mktemp -d "${TMPDIR:-/tmp}/sr-subjects.XXXXXX")" || refuse "cannot make a directory for the subjects' fingerprints"
  fail() { rm -rf "$tmp"; refuse "$1"; }
  lines="$(jq -c '.[] | .text | tojson' <<<"$arr")" || fail "the subjects' fingerprints could not be prepared, so no subject could be made"
  while IFS= read -r line; do printf '%s\n' "$line" >"$tmp/$i" || fail "a subject's fingerprint input could not be written"; i=$((i + 1)); done <<<"$lines"
  paths="$(seq 0 $((n - 1)) | sed "s|^|$tmp/|")" || fail "the subjects' fingerprint inputs could not be named"
  hashes="$(git hash-object --stdin-paths <<<"$paths")" || fail "the subjects' fingerprints could not be computed, so no subject could be made"
  [ "$(grep -c . <<<"$hashes")" -eq "$n" ] || fail "the subjects' fingerprints came back the wrong number"
  out="$(jq -Rn --slurpfile s <(printf '%s' "$arr") '[inputs] as $h | [range(0; $s[0] | length) as $i | $s[0][$i] | {id, files} + (if .text == "" then {} else {fingerprint: $h[$i]} end)]' <<<"$hashes")" ||
    fail "the subjects could not be built"
  rm -rf "$tmp"
  printf '%s\n' "$out"
}

# changed_paths — every path the changeset selected, one per line.
changed_paths() { cs '.changeset.files[].path'; }
