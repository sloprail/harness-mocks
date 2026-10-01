#!/bin/sh
# A hook that fails non-blockingly with two lines of stderr.
cat >/dev/null
printf 'first line of stderr\nsecond line of stderr\n' >&2
exit 1
