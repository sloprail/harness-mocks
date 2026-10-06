#!/usr/bin/env bash
# The mock capability-rigor judge of the statement-scope cases (see scope-lib.sh). Its rule is the rubric's:
# each clause of the statement (the words after "proves ") must be in a test or in the events of a run marked
# replays="true" (a green replay); a run on the replay exception list (replays="false") proves nothing, and what
# only the doc page says is never demanded.
input="$(cat)"
cd "${SR_TREE:-.}" 2>/dev/null || true
for need in "not a checklist" "Replays count" "outside the statement's scope" 'replays="true"'; do
  printf '%s\n' "$input" | grep -qF "$need" || { echo "{\"pass\":false,\"reasoning\":\"the rubric lost: $need\"}"; exit 0; }
done
clauses="$(printf '%s\n' "$input" | sed -n 's/.*<statement>.*proves \(.*\)<\/statement>.*/\1/p')"
tests="$(printf '%s\n' "$input" | grep -o '<test path="[^"]*"' | sed 's/^<test path="//; s/"$//')"
events="$(printf '%s\n' "$input" | awk '/<run /{ok = ($0 ~ /replays="true"/)} ok && match($0, /<sample events="[^"]*"/) {print substr($0, RSTART + 16, RLENGTH - 17)}')"
for c in $clauses; do
  # shellcheck disable=SC2086
  grep -qF "$c" $tests $events 2>/dev/null || { echo "{\"pass\":false,\"reasoning\":\"no test or replay proves the clause $c\"}"; exit 0; }
done
echo '{"pass":true,"reasoning":""}'
