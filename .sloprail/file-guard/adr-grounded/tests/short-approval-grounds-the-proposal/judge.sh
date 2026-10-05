#!/usr/bin/env bash
# A mock of the judge for a short approval. Like the real one it is handed the quote and where it sits
# (source="<transcript>:<line>"), and reads the record itself: what the quote approves is what the assistant
# said anywhere before that line. The quote must be an approval, and every subject (id="...") the change
# touches must be named there.
in=$(cat)
quote=$(printf '%s' "$in" | grep -o '<quote>[^<]*</quote>' | head -1)
source=$(printf '%s' "$in" | grep -o 'source="[^"]*"' | head -1 | sed 's/^source="//; s/"$//')
file=${source%:*}
line=${source##*:}
ids=$(printf '%s' "$in" | grep -o '<subject id="[^"]*"' | sed 's/^<subject id="//; s/"$//')
if ! printf '%s' "$quote" | grep -qi 'lgtm'; then
  echo '{"pass":false,"reasoning":"the quote is not an approval"}'; exit 0
fi
if [ ! -r "$file" ] || [ -z "$ids" ]; then
  echo '{"pass":false,"reasoning":"could not read what the quote answered, or no subject was handed"}'; exit 0
fi
said=$(head -n "$((line - 1))" "$file" | jq -r 'select(.type=="assistant")|.message.content[]?|select(.type=="text")|.text')
for t in $ids; do
  if ! printf '%s' "$said" | grep -q "$t"; then
    echo "{\"pass\":false,\"reasoning\":\"lgtm approved what the assistant proposed, and that did not include $t\"}"; exit 0
  fi
done
echo '{"pass":true,"reasoning":"the short approval grounds exactly what the assistant messages before it proposed"}'
