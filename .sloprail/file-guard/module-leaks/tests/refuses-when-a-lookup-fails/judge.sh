#!/usr/bin/env bash
# A mock judge that passes whatever it is handed, provided it is handed something: these cases are about
# the lookups before the judge, so a prepare that fed it nothing would fail here.
IFS= read -r -d '' input
if [ -z "$input" ]; then
  echo '{"pass":false,"reasoning":"the judge was handed no input"}'
else
  echo '{"pass":true,"reasoning":""}'
fi
