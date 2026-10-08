#!/usr/bin/env bash
# A mock of the adr-matches-sloprails judge that decides from its input: the rendered prompt names each ADR as
# <adr id="…" path="…"/> and each file of each linked rule as <rule-file rule="…" path="…" also-enforces="…"/> (project
# paths, read from $SR_TREE, the committed tree). An ADR whose Decision says FORBIDDEN is enforced only if some file
# of its linked rules says FORBIDDEN too; otherwise it is a decision no rule checks and the ADR fails, named.
# Any other ADR passes.
input="$(cat)"
bad=""
subject=""
decides=0
enforced=0
flush() {
  if [ -n "$subject" ] && [ "$decides" = 1 ] && [ "$enforced" = 0 ]; then bad="$bad adr/$subject"; fi
}
while IFS= read -r line; do
  case "$line" in
    '<adr '*)
      flush
      subject="$(printf '%s' "$line" | sed 's/^<adr id="\([^"]*\)".*/\1/')"
      path="$(printf '%s' "$line" | sed 's/.* path="\([^"]*\)".*/\1/')"
      decides=0; enforced=0
      sed -n '/^## Decision/,$p' "$SR_TREE/$path" | grep -q 'FORBIDDEN' && decides=1 ;;
    '<rule-file '*)
      path="$(printf '%s' "$line" | sed 's/.* path="\([^"]*\)".*/\1/')"
      grep -q 'FORBIDDEN' "$SR_TREE/$path" && enforced=1 ;;
  esac
done < <(printf '%s\n' "$input" | grep -E '^<(adr|rule-file) ')
flush
if [ -n "$bad" ]; then
  echo "{\"pass\":false,\"reasoning\":\"a Decision no linked rule checks:$bad says FORBIDDEN and no linked rule does\"}"
else
  echo '{"pass":true,"reasoning":""}'
fi
