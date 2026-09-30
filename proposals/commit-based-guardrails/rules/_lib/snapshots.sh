#!/usr/bin/env bash
# Snapshots of a real harness, frozen at one version. Source after changeset.sh.
#
#   <harness>-mock/snapshots/
#     MANIFEST.yaml          version: <the one freeze point>
#                            docs: {<page URL>: {version, sha256}}
#     docs/<host>/<path>.md  a doc page, copied at that version; its path is a
#                            pure function of its URL (doc_path), so no lookup
#     runs/<name>/           a recorded real scenario
#       run.yaml             version, command
#       setup/               what makes it this scenario (settings, hooks, prompt)
#       samples/<YYYYMMDD-HHMMSS>/
#         events.jsonl       the normalized event sequence (hook payloads + stream frames)
#         …                  the raw capture (payloads.jsonl, stream.jsonl, transcript/)
#
# A capability cites them per harness: docs by the page's full URL plus
# #anchor (the copy is at doc_path of that URL), runs by their
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

# doc_path URL — where the copy of a doc page lives, relative to the snapshots
# dir: https://code.claude.com/docs/en/hooks#stop → docs/code.claude.com/docs/en/hooks.md
doc_path() {
  local u="${1%%#*}"; u="${u%%\?*}"; u="${u#https://}"; u="${u#http://}"; u="${u%/}"
  printf 'docs/%s.md' "$u"
}
# doc_url PATH — the inverse: docs/<host>/<path>.md → https://<host>/<path>.
doc_url() { local p="${1#docs/}"; printf 'https://%s' "${p%.md}"; }

# doc_file HARNESS URL — doc_path of URL, if that copy exists. Exit 1 if not.
doc_file() {
  local f; f="$(doc_path "$2")"
  [ -f "$(snap_dir "$1")/$f" ] || return 1
  printf '%s' "$f"
}

# doc_ref_section HARNESS URL#ANCHOR — the cited section's text from the snapshot.
doc_ref_section() {
  local f
  f="$(doc_file "$1" "$2")" || return 1
  doc_section "$(snap_dir "$1")/$f" "${2#*#}"
}
