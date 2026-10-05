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
# A lookup that fails refuses (leaks-lib.sh): a failed one is never "no candidates", which would
# judge nothing. What grows (the candidates, the changed paths, the exceptions) goes to jq by file,
# never as one command-line argument (Linux caps one at 128 KB).
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/modules.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/subjects.sh"
. "${SR_GUARDRAIL_DIR:-.}/leaks-lib.sh"
load_modules; load_adrs; leak_setup; leak_prefetch
printf '%s' "$CHANGED" >"$LEAK_WORK/changed"
printf '%s' "$EXC" >"$LEAK_WORK/exc-text"
mlist="$(jq -c '.[]' <<<"$MODULES")" || refuse "could not list the modules, so their leaks cannot be found"
out=""
while IFS= read -r m; do
  [ -n "$m" ] || continue
  dir="$(jq -r '.dir' <<<"$m")" || refuse "could not read a module's directory, so its leaks cannot be found"
  leak_left "$m" || continue
  nleft="$(jq 'length' <<<"$LEFT")" || refuse "could not count the candidates of $dir"
  [ "$nleft" -gt 0 ] || continue
  printf '%s' "$LEFT" >"$LEAK_WORK/left"
  entry="$(jq -nc --arg d "$dir" --slurpfile left "$LEAK_WORK/left" --rawfile changed "$LEAK_WORK/changed" --rawfile exc "$LEAK_WORK/exc-text" \
    '$left[0] as $left | ($changed | split("\n")) as $c
     | {id: $d,
        files: ([$left[].path, "\($d)/module.yaml", "\($d)/candidates.sh"] | map(select(. as $p | $c | index($p)))),
        deps: ["\($d)/module.yaml", "\($d)/candidates.sh"],
        extra: ("exceptions:" + $exc + "\nleft:" + ($left | map("\(.path):\(.line):\(.text)") | join("\n")))}')" ||
    refuse "could not build the subject of module $dir"
  out="$out$entry"$'\n'
done <<<"$mlist"
subs="$(printf '%s' "$out" | jq -sc .)" || refuse "the subjects could not be collected into one list"
sub_finish no-leak-candidates "$subs"
