#!/usr/bin/env bash
# A mock of the adr-well-formed judge that decides from its input: the rendered prompt names each ADR as
# <adr id="…" path="…" diff="…"/> (path is a project path, read from $SR_TREE, the committed tree). The judge asks
# one thing only: that a Decision bullet names where the decision applies (a `path`, package or module in code
# format, or the Concern it sits under does). A bullet that names no place fails, named with the ADR; a hedge
# ("prefer") or a missing mechanism is not its business.
input="$(cat)"
bad=""
while IFS= read -r line; do
  id="$(printf '%s' "$line" | sed 's/^<adr id="\([^"]*\)".*/\1/')"
  path="$(printf '%s' "$line" | sed 's/.* path="\([^"]*\)".*/\1/')"
  [ -f "$SR_TREE/$path" ] || continue
  while IFS= read -r bullet; do
    printf '%s\n' "$bullet" | grep -q '`' || bad="$bad adr/$id names no place in '${bullet#- }'"
  done < <(sed -n '/^## Decision/,$p' "$SR_TREE/$path" | grep '^- ')
done < <(printf '%s\n' "$input" | grep '^<adr ')
if [ -n "$bad" ]; then
  echo "{\"pass\":false,\"reasoning\":\"a Decision bullet does not say where it applies:$bad\"}"
else
  echo '{"pass":true,"reasoning":""}'
fi
