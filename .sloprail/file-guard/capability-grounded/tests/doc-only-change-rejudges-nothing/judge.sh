#!/usr/bin/env bash
# A mock of the capability-grounded judge that decides from its input (the rendered prompt carries each
# cited doc ref as a ref="…" attribute, whatever the diff holds): a capability citing a section that shows nothing of it (#undocumented) is refused, any other passes.
input="$(cat)"
if printf '%s\n' "$input" | grep -q 'ref="[^"]*#undocumented"'; then
  echo '{"pass":false,"reasoning":"the statement claims a part no cited doc or run shows"}'
else
  echo '{"pass":true,"reasoning":""}'
fi
