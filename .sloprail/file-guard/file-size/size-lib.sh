#!/usr/bin/env bash
# adr/file-size's rule, shared by both halves: the file-guard (committed
# changesets, size.sh) and the gate (a Write/Edit before it lands,
# ../../gate/file-size/size.sh). No event-kind logic here: each entry reads its
# own input and calls size_problem. Source it; do not run it.
#
# Needs: refuse (the entry's), and SR_GUARDRAIL_DIR set to the calling rule's folder.

. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"

# size_load — read the limits and per-file ceilings from the ADR(s) that link
# the calling rule, so the decision and its check cannot disagree. A ceiling is
# an absolute number, not "no bigger than before": a legacy file a move-only
# split CREATED has no "before", and must still be held to its size.
size_load() {
  local fm
  load_adrs "$(rule_qname)"
  fm="$(jq -c '[.[] | .frontmatter | select(.limits)][0] // empty' <<<"$ADRS")"
  [ -n "$fm" ] || refuse "no ADR linking $(rule_qname) declares limits, so file sizes cannot be checked"
  SIZE_LIM_GO="$(jq -r '.limits.go' <<<"$fm")"
  SIZE_LIM_TEST="$(jq -r '.limits.go_test' <<<"$fm")"
  SIZE_CEILINGS="$(jq -c '[.[] | .frontmatter.ceilings // {}] | add // {}' <<<"$ADRS")"
}

# size_problem PATH CONTENT — prints one sentence if CONTENT is too large for
# PATH, nothing otherwise.
size_problem() {
  local path="$1" content="$2" limit n c
  case "$path" in *_test.go) limit="$SIZE_LIM_TEST" ;; *.go) limit="$SIZE_LIM_GO" ;; *) return 0 ;; esac
  if [ -z "$content" ]; then n=0; else n="$(printf '%s\n' "$content" | awk 'END { print NR }')"; fi
  c="$(jq -r --arg p "$path" '.[$p] // empty' <<<"$SIZE_CEILINGS")"
  if [ -n "$c" ]; then
    [ "$n" -le "$c" ] || echo "$path holds $n lines, over its legacy ceiling of $c: split it with a move-only refactor instead of growing it"
    return 0
  fi
  [ "$n" -le "$limit" ] || echo "$path would be $n lines, over the limit of $limit: split it by responsibility into smaller files"
}
