#!/usr/bin/env bash
# A mock of the invariant-grounded judge that decides from its input: the rendered prompt names each invariant as
# <invariant id="…" status="…" before="…" after="…"/> (after is a project path, read from $SR_TREE, the committed
# tree) and lists the cited words as <quote>…</quote>. A statement that states a number of seconds the words do not
# is broader than the words and fails (naming the invariant); any other statement passes.
input="$(cat)"
quotes="$(printf '%s\n' "$input" | grep -o '<quote>[^<]*</quote>')"
bad=""
while IFS= read -r line; do
  id="$(printf '%s' "$line" | sed 's/^<invariant id="\([^"]*\)".*/\1/')"
  after="$(printf '%s' "$line" | sed 's/.* after="\([^"]*\)".*/\1/')"
  [ -n "$after" ] && [ -f "$SR_TREE/$after" ] || continue
  if grep -q 'seconds' "$SR_TREE/$after" && ! printf '%s' "$quotes" | grep -q 'seconds'; then bad="$bad $id"; fi
done < <(printf '%s\n' "$input" | grep '^<invariant ')
if [ -n "$bad" ]; then
  echo "{\"pass\":false,\"reasoning\":\"the statement of$bad adds a number of seconds the words quoted do not state\"}"
else
  echo '{"pass":true,"reasoning":""}'
fi
