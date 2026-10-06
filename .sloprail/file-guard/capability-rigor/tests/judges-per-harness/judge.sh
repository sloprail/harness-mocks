#!/usr/bin/env bash
# A mock of the capability-rigor judge that decides from its input (the rendered prompt carries each
# cited doc ref as a ref="…" attribute, one <subject> per (capability, harness) pair it is handed): tests proving
# a capability that cites a section showing nothing of it (#undocumented) are refused, any other passes.
input="$(cat)"
if printf '%s\n' "$input" | grep -q 'ref="[^"]*#undocumented"'; then
  echo '{"pass":false,"reasoning":"the tests do not drive the mock into what the recording shows"}'
else
  echo '{"pass":true,"reasoning":""}'
fi
