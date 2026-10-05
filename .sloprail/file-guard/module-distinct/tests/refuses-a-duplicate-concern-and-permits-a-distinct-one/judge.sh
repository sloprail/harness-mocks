#!/usr/bin/env bash
# A mock of the distinct-concern judge that decides from its input: the prompt lists the changed module's
# <concern> first, then each other module's; the same concern twice is the same responsibility.
IFS= read -r -d '' input
mine="$(printf '%s\n' "$input" | sed -n 's|^<concern>\(.*\)</concern>$|\1|p' | head -1)"
if [ -n "$mine" ] && [ "$(printf '%s\n' "$input" | grep -c -F -x "<concern>$mine</concern>")" -gt 1 ]; then
  echo '{"pass":false,"reasoning":"the module'"'"'s concern duplicates another module'"'"'s"}'
else
  echo '{"pass":true,"reasoning":"every other module is distinct"}'
fi
