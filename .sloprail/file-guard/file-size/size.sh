#!/usr/bin/env bash
# adr/file-size's deterministic check. It runs in two places:
#   - as this file-guard, on a Changeset (every changed .go file, committed bytes);
#   - as gate/adr-0001-file-size, on a PreFileCreate/PreFileUpdate (one file,
#     before the write).
set -uo pipefail
payload="$(cat)"
refuse() { jq -n --arg r "$1" '{reason: $r}'; exit 1; }
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"

# Limits and exceptions come from the ADR(s) that link this rule, so the
# decision and its check cannot disagree. Both the gate and the file-guard of
# this name are linked from adr/file-size.
load_adrs "$(rule_qname)"
fm="$(jq -c '[.[] | .frontmatter | select(.limits)][0] // empty' <<<"$ADRS")"
[ -n "$fm" ] || refuse "no ADR linking $(rule_qname) declares limits, so file sizes cannot be checked"
lim_go="$(jq -r '.limits.go' <<<"$fm")"
lim_test="$(jq -r '.limits.go_test' <<<"$fm")"
exceptions="$(jq -r '[.[] | .frontmatter.exceptions // [] | .[]] | .[]' <<<"$ADRS")"

lines() { if [ -z "$1" ]; then echo 0; else printf '%s\n' "$1" | awk 'END { print NR }'; fi; }

# check PATH OLD NEW — prints a problem, or nothing.
check() {
  local path="$1" old="$2" new="$3" limit n o
  case "$path" in *_test.go) limit="$lim_test" ;; *.go) limit="$lim_go" ;; *) return 0 ;; esac
  n="$(lines "$new")"
  if printf '%s\n' "$exceptions" | grep -Fxq -- "$path"; then
    o="$(lines "$old")"
    [ "$n" -le "$o" ] || echo "$path is on the file-size ADR's exception list and may not grow ($o → $n lines): split it with a move-only refactor instead"
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
      p="$(check "$(jq -r '.path' <<<"$f")" "$(jq -r '.oldContent // ""' <<<"$f")" "$(jq -r '.newContent // ""' <<<"$f")")"
      [ -z "$p" ] || problems="${problems}- $p"$'\n'
    done < <(jq -c '.changeset.files[] | select(.status != "D")' <<<"$payload")
    ;;
  PreFileCreate | PreFileUpdate)
    # An edit whose result cannot be predicted is left to the commit check.
    [ "$(jq -r '.event.resultKnown' <<<"$payload")" = "true" ] || exit 0
    p="$(check "$(jq -r '.event.path' <<<"$payload")" "$(jq -r '.event.oldContent // ""' <<<"$payload")" "$(jq -r '.event.newContent // ""' <<<"$payload")")"
    [ -z "$p" ] || problems="- $p"$'\n'
    ;;
  *) refuse "the file-size ADR's size check got an unexpected event kind '$kind'" ;;
esac
[ -z "$problems" ] && exit 0
refuse "adr/file-size (Go files stay small):
${problems}"
