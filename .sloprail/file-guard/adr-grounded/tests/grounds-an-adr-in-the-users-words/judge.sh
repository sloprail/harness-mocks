#!/usr/bin/env bash
# A mock of the adr-grounded judge that decides from its input: the rendered prompt names each ADR as
# <adr id="…" before="…" after="…"/> (after is a project path, read from $SR_TREE, the committed tree) and lists the
# cited words as <quote>…</quote>. An ADR that decides "forever" when the words do not say so decides more than the
# words and fails (naming the ADR); any other ADR passes.
input="$(cat)"
quotes="$(printf '%s\n' "$input" | grep -o '<quote>[^<]*</quote>')"
bad=""
while IFS= read -r line; do
  id="$(printf '%s' "$line" | sed 's/^<adr id="\([^"]*\)".*/\1/')"
  after="$(printf '%s' "$line" | sed 's/.* after="\([^"]*\)".*/\1/')"
  [ -n "$after" ] && [ -f "$SR_TREE/$after" ] || continue
  if grep -q 'forever' "$SR_TREE/$after" && ! printf '%s' "$quotes" | grep -q 'forever'; then bad="$bad adr/$id"; fi
done < <(printf '%s\n' "$input" | grep '^<adr ')
if [ -n "$bad" ]; then
  echo "{\"pass\":false,\"reasoning\":\"$bad decides more than the words quoted: it says forever\"}"
else
  echo '{"pass":true,"reasoning":""}'
fi
