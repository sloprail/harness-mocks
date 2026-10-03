#!/usr/bin/env bash
# subjects: one per module that has candidates left to judge (leaks-lib.sh: its own search, scoped
# to the lines this range adds, minus its home, tests and exceptions: what find-leaks.sh hands the
# judge). A module with none has nothing to judge and no subject.
#   files        the changed files the leftover candidates sit in, and the module's own module.yaml
#                and candidates.sh when they changed
#   fingerprint  what the judge reads beyond them: the leftover candidates themselves (a candidate
#                in a file the range did not touch, when module.yaml or candidates.sh changed), the
#                module's boundary and concern (module.yaml), its candidates.sh and the ADRs'
#                exceptions.
# Adding code that matches module A's search leaves module B's subject as it was.
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
. "${SR_GUARDRAIL_DIR:-.}/leaks-lib.sh"
load_modules; load_adrs; leak_setup; leak_prefetch
out=""
while IFS= read -r m; do
  [ -n "$m" ] || continue
  dir="$(jq -r '.dir' <<<"$m")"
  leak_left "$m" || continue
  [ "$(jq 'length' <<<"$LEFT")" -gt 0 ] || continue
  out="$out$(jq -nc --arg d "$dir" --argjson left "$LEFT" --arg changed "$CHANGED" --arg exc "$EXC" \
    '($changed | split("\n")) as $c
     | {id: $d,
        files: ([$left[].path, "\($d)/module.yaml", "\($d)/candidates.sh"] | map(select(. as $p | $c | index($p)))),
        deps: ["\($d)/module.yaml", "\($d)/candidates.sh"],
        extra: ("exceptions:" + $exc + "\nleft:" + ($left | map("\(.path):\(.line):\(.text)") | join("\n")))}')"$'\n'
done < <(jq -c '.[]' <<<"$MODULES")
sub_finish no-leak-candidates "$(printf '%s' "$out" | jq -sc .)"
