#!/usr/bin/env bash
# A mock of the adr-well-formed judge that decides from its input: the rendered prompt names each ADR as
# <adr id="…" path="…" diff="…"/> (path is a project path, read from $SR_TREE, the committed tree). A Decision bullet
# that hedges ("prefer", "will migrate") is not a decision rules can enforce: the ADR fails, named with the word.
# Any other ADR passes.
input="$(cat)"
bad=""
while IFS= read -r line; do
  id="$(printf '%s' "$line" | sed 's/^<adr id="\([^"]*\)".*/\1/')"
  path="$(printf '%s' "$line" | sed 's/.* path="\([^"]*\)".*/\1/')"
  [ -f "$SR_TREE/$path" ] || continue
  word="$(sed -n '/^## Decision/,$p' "$SR_TREE/$path" | grep -o -i -E 'prefer|will migrate' | head -1)"
  [ -n "$word" ] && bad="$bad adr/$id says '$word'"
done < <(printf '%s\n' "$input" | grep '^<adr ')
if [ -n "$bad" ]; then
  echo "{\"pass\":false,\"reasoning\":\"a Decision bullet is not enforceable:$bad\"}"
else
  echo '{"pass":true,"reasoning":""}'
fi
