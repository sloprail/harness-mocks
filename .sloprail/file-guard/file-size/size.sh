#!/usr/bin/env bash
# adr/file-size's deterministic check. It runs in two places:
#   - as this file-guard, on a Changeset (every changed .go file, committed bytes);
#   - as gate/file-size, on a PreFileCreate/PreFileUpdate (one file,
#     before the write).
set -uo pipefail
payload="$(cat)"
refuse() { jq -n --arg r "$1" '{reason: $r}'; exit 1; }
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"

# Limits and per-file ceilings come from the ADR(s) that link this rule, so the
# decision and its check cannot disagree. Both the gate and the file-guard of
# this name are linked from adr/file-size. A ceiling is an absolute number, not
# "no bigger than before": a legacy file a move-only split CREATED has no
# "before", and must still be held to its size.
load_adrs "$(rule_qname)"
fm="$(jq -c '[.[] | .frontmatter | select(.limits)][0] // empty' <<<"$ADRS")"
[ -n "$fm" ] || refuse "no ADR linking $(rule_qname) declares limits, so file sizes cannot be checked"
lim_go="$(jq -r '.limits.go' <<<"$fm")"
lim_test="$(jq -r '.limits.go_test' <<<"$fm")"
ceilings="$(jq -c '[.[] | .frontmatter.ceilings // {}] | add // {}' <<<"$ADRS")"

lines() { if [ -z "$1" ]; then echo 0; else printf '%s\n' "$1" | awk 'END { print NR }'; fi; }

# check PATH NEW — prints a problem, or nothing.
check() {
  local path="$1" new="$2" limit n c
  case "$path" in *_test.go) limit="$lim_test" ;; *.go) limit="$lim_go" ;; *) return 0 ;; esac
  n="$(lines "$new")"
  c="$(jq -r --arg p "$path" '.[$p] // empty' <<<"$ceilings")"
  if [ -n "$c" ]; then
    [ "$n" -le "$c" ] || echo "$path holds $n lines, over its legacy ceiling of $c: split it with a move-only refactor instead of growing it"
    return 0
  fi
  [ "$n" -le "$limit" ] || echo "$path would be $n lines, over the limit of $limit: split it by responsibility into smaller files"
}

kind="$(jq -r '.event.kind // ""' <<<"$payload")"
problems=""
case "$kind" in
  Changeset)
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      p="$(check "$(jq -r '.path' <<<"$f")" "$(jq -r '.newContent // ""' <<<"$f")")"
      [ -z "$p" ] || problems="${problems}- $p"$'\n'
    done < <(jq -c '.changeset.files[] | select(.status != "D")' <<<"$payload")
    ;;
  PreFileCreate | PreFileUpdate)
    # An edit whose result cannot be predicted is left to the commit check.
    [ "$(jq -r '.event.resultKnown' <<<"$payload")" = "true" ] || exit 0
    p="$(check "$(jq -r '.event.path' <<<"$payload")" "$(jq -r '.event.newContent // ""' <<<"$payload")")"
    [ -z "$p" ] || problems="- $p"$'\n'
    ;;
  *) refuse "the file-size ADR's size check got an unexpected event kind '$kind'" ;;
esac
[ -z "$problems" ] && exit 0
refuse "adr/file-size (Go files stay small):
${problems}"
