#!/usr/bin/env bash
# A mock of the adr-conformance judge that decides from its input: the rendered prompt names each change as
# <change path="…" … diff="…"/>, the diff being a file outside the project. A diff that adds a line saying
# FORBIDDEN breaks the ADR's one decision (named in the reasoning with the file), any other passes.
bad=""
while IFS= read -r line; do
  path="$(printf '%s' "$line" | sed 's/^<change path="\([^"]*\)".*/\1/')"
  diff="$(printf '%s' "$line" | sed 's/.* diff="\([^"]*\)".*/\1/')"
  if [ -f "$diff" ] && grep -q '^+.*FORBIDDEN' "$diff"; then bad="$bad $path"; fi
done < <(grep '^<change ')
if [ -n "$bad" ]; then
  echo "{\"pass\":false,\"reasoning\":\"quiet #1:$bad breaks\"}"
else
  echo '{"pass":true,"reasoning":""}'
fi
