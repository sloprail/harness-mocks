#!/usr/bin/env bash
# A mock of the capability-rigor judge that always passes: the case asserts the rule's own input checks, never a verdict.
cat >/dev/null
echo '{"pass":true,"reasoning":""}'
