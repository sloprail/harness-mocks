#!/usr/bin/env bash
# A mock of the invariant-rigor judge that decides from its input: the rendered prompt names each invariant's
# proving tests as <test path="…"/> (project paths, read from $SR_TREE, the committed tree). A test that asserts
# nothing (no t.Fatal in it) does not prove the statement and fails, naming the test; if every test asserts, it passes.
input="$(cat)"
bad=""
while IFS= read -r path; do
  [ -f "$SR_TREE/$path" ] || { bad="$bad $path (missing)"; continue; }
  grep -q 't\.Fatal' "$SR_TREE/$path" || bad="$bad $path"
done < <(printf '%s\n' "$input" | grep -o '<test path="[^"]*"/>' | sed 's/^<test path="//; s/"\/>$//')
if [ -n "$bad" ]; then
  echo "{\"pass\":false,\"reasoning\":\"these tests assert nothing the statement says:$bad\"}"
else
  echo '{"pass":true,"reasoning":""}'
fi
