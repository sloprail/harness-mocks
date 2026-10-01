#!/usr/bin/env bash
# Snapshots of a real harness, frozen at one version. Source after changeset.sh.
#
#   <harness>-mock/snapshots/
#     MANIFEST.yaml          version: <the one freeze point>
#                            docs: {<page URL>: {version, sha256}}  ← only the
#                            hash of each doc page is committed, never its text
#     runs/<name>/           a recorded real scenario
#       run.yaml             version, command
#       setup/               what makes it this scenario (settings, hooks, prompt)
#       samples/<YYYYMMDD-HHMMSS>/
#         events.jsonl       the normalized event sequence (hook payloads + stream frames)
#         …                  the raw capture (payloads.jsonl, stream.jsonl, transcript/)
#
# A doc page's TEXT is never committed (it is the harness vendor's). Its copy
# lives in a cache under the repository's git dir, keyed by the sha256 the
# MANIFEST records, and is fetched from <url>.md on a miss. A fetched page
# whose hash differs from the MANIFEST's is not the page that was frozen: the
# doc changed since capture, and the snapshot is stale.
#
# A capability cites them per harness: docs by the page's full URL plus
# #anchor, runs by their
# repo-relative path (<harness>-mock/snapshots/runs/<name>). A run is a
# scenario; its captures are the timestamped samples inside it.

snap_dir() { printf '%s/%s-mock/snapshots' "$SR_TREE" "$1"; }

# slug HEADING — the anchor a docs site gives a heading: lowercase, spaces to
# dashes, anything but [a-z0-9-] dropped.
slug() { printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[[:space:]]+/-/g; s/[^a-z0-9-]//g'; }

# doc_section FILE ANCHOR — prints the section under the heading whose slug is
# ANCHOR, up to the next heading of the same or a higher level. Exit 1 if absent.
doc_section() {
  local file="$1" anchor="$2"
  [ -f "$file" ] || return 1
  awk -v want="$anchor" '
    function slugify(h) { h = tolower(h); gsub(/[[:space:]]+/, "-", h); gsub(/[^a-z0-9-]/, "", h); return h }
    /^#+[[:space:]]/ {
      lvl = match($0, /[^#]/) - 1; h = substr($0, lvl + 1); sub(/^[[:space:]]+/, "", h)
      if (on && lvl <= onlvl) exit
      if (!on && slugify(h) == want) { on = 1; onlvl = lvl; found = 1 }
    }
    on { print }
    END { exit found ? 0 : 1 }' "$file"
}

# doc_cache_dir — where fetched doc pages are cached: inside the repository's
# common git dir (shared by every worktree, never committed).
doc_cache_dir() { printf '%s/sloprail-doc-cache' "$(git -C "$SR_TREE" rev-parse --path-format=absolute --git-common-dir)"; }

# doc_sha HARNESS URL — the sha256 the MANIFEST froze for URL (anchor dropped).
doc_sha() {
  local m="$(snap_dir "$1")/MANIFEST.yaml" u="${2%%#*}"
  [ -f "$m" ] || return 1
  U="$u" yq -r '.docs[strenv(U)].sha256 // ""' "$m" 2>/dev/null | grep -E '^[0-9a-f]{64}$'
}

# DOC_ERROR — why doc_copy failed, for the caller's refusal.
# doc_copy HARNESS URL — prints the path of the cached copy of URL's frozen
# page, fetching it on a miss. Exit 1 (DOC_ERROR set) when the MANIFEST does
# not freeze the URL, the page cannot be fetched, or the live page's hash
# differs from the frozen one (stale).
doc_copy() {
  local sha dir f tmp got u="${2%%#*}"
  DOC_ERROR=""
  sha="$(doc_sha "$1" "$u")" || { DOC_ERROR="no snapshot in $1-mock/snapshots/MANIFEST.yaml freezes $u"; return 1; }
  dir="$(doc_cache_dir)"; f="$dir/$sha.md"
  [ -f "$f" ] && { printf '%s' "$f"; return 0; }
  mkdir -p "$dir" && tmp="$(mktemp "$dir/fetch.XXXXXX")" || { DOC_ERROR="the doc cache $dir is not writable"; return 1; }
  if ! curl -fsSL --max-time 30 "$u.md" -o "$tmp"; then
    rm -f "$tmp"; DOC_ERROR="could not fetch $u.md to check it against its frozen hash"; return 1
  fi
  got="$(shasum -a 256 "$tmp" | cut -d' ' -f1)"
  if [ "$got" != "$sha" ]; then
    rm -f "$tmp"; DOC_ERROR="$u changed since it was captured (its hash is no longer the frozen one): re-capture it with capture.sh doc $u, and re-check what cites it"; return 1
  fi
  mv "$tmp" "$f" && printf '%s' "$f"
}

# doc_ref_section HARNESS URL#ANCHOR — the cited section's text, from the
# frozen page. Exit 1 when the page cannot be had (DOC_ERROR) or has no such section.
doc_ref_section() {
  local f
  f="$(doc_copy "$1" "$2")" || return 1
  doc_section "$f" "${2#*#}"
}
