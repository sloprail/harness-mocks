#!/usr/bin/env bash
# A mock of the leak-or-use judge that decides from its input: the prompt names the file listing the
# candidates (matches="..."); a candidate that reimplements the module's logic is a leak, any other is a use.
IFS= read -r -d '' input
list="$(printf '%s\n' "$input" | sed -n 's/.*matches="\([^"]*\)".*/\1/p' | head -1)"
if [ -n "$list" ] && grep -q 'reimplements' "$list"; then
  echo '{"pass":false,"reasoning":"a leak: the module'"'"'s logic is reimplemented outside its home"}'
else
  echo '{"pass":true,"reasoning":"every candidate is a use of the module'"'"'s API"}'
fi
