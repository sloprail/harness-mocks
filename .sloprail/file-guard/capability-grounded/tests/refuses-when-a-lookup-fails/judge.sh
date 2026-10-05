#!/usr/bin/env bash
# A mock of the capability-grounded judge that always passes: the case asserts the rule's own lookups, never a verdict.
cat >/dev/null
echo '{"pass":true,"reasoning":""}'
