#!/usr/bin/env bash
# Shared helpers for the commit-based checks. Source it; do not run it.
#
# A check reads ONE payload on stdin. Capture it into $payload before sourcing:
#
#   payload="$(cat)"
#   . "$SR_GUARDRAIL_DIR/../../_lib/changeset.sh"
#
# Payload shape (the Changeset event, see the design note):
#   .event.kind == "Changeset"
#   .changeset.{base,head,commits[],files[],others[],citations[]}
#   .changeset.files[] = {path,status,oldPath,oldContent,newContent,oldMarkers,newMarkers,diff}
#   .subject = {id, files[], context{}}   (only when the rule declares subjects:)
# Checks read the committed tree at $SR_TREE, never the working tree.

# cs JQ — a raw jq read of the payload.
cs() { printf '%s' "$payload" | jq -r "$1"; }

# cs_json JQ — a compact JSON read of the payload.
cs_json() { printf '%s' "$payload" | jq -c "$1"; }

# refuse MESSAGE — the refusal contract: {"reason"} on stdout, exit 1.
refuse() {
  jq -n --arg r "$1" '{reason: $r}'
  exit 1
}

# The committed tree this check judges, checked once, here, at the top level.
# A check that cannot see the commit must not read the working tree in its
# place: fail closed. (A refuse inside $(…) would only exit the subshell and
# leave its JSON in a variable, so no helper below refuses from inside one:
# they set variables instead.)
[ -n "${SR_TREE:-}" ] && [ -d "${SR_TREE:-}" ] ||
  refuse "SR_TREE is not set, so the committed tree cannot be read; this rule only judges commits"
tree() { printf '%s' "$SR_TREE"; }

# load_markers KIND — sets MARKERS to every `sr:<KIND> <fqn>` marker in the
# committed tree, one "path<TAB>fqn" per line. Uses the engine's marker grammar:
# the whole line is the marker, after a //, # or -- leader; the fqn is a quoted
# or bare token. git grep exits 1 on no match (fine) and >1 on an error (refuse).
load_markers() {
  local kind="$1" out rc
  MARKERS=""
  out="$(git -C "$SR_TREE" grep -n -I -E \
    "^[[:space:]]*(//|#|--)[[:space:]]*sr:${kind}[[:space:]]+(\"[^\"]*\"|[^[:space:]\"][^[:space:]]*)[[:space:]]*$" \
    -- . ':!proposals/**' 2>&1)"
  rc=$?
  [ "$rc" -le 1 ] || refuse "could not search the committed tree for sr:${kind} markers: $out"
  [ -n "$out" ] || return 0
  MARKERS="$(printf '%s\n' "$out" | sed -E \
    "s#^([^:]+):[0-9]+:[[:space:]]*(//|\#|--)[[:space:]]*sr:${kind}[[:space:]]+\"?([^\"[:space:]]+)\"?[[:space:]]*\$#\1\t\3#")"
}

# load_yaml FILE — sets YAML to a committed-tree YAML file as JSON ("null" if absent).
load_yaml() {
  local f="$SR_TREE/$1"
  YAML=null
  [ -f "$f" ] || return 0
  YAML="$(yq -o=json '.' "$f" 2>/dev/null)" || refuse "$1 is not valid YAML"
}

# yaml_str_json STRING — YAML text (e.g. a file's oldContent) as JSON.
yaml_str_json() {
  [ -n "$1" ] || { printf 'null'; return 0; }
  printf '%s' "$1" | yq -o=json '.' 2>/dev/null || printf 'null'
}

# changed_file PATH — the changeset's entry for PATH, or "null".
changed_file() { cs_json "[.changeset.files[] | select(.path == \"$1\")][0] // null"; }
