#!/usr/bin/env bash
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/confine.sh"
ADR_TITLE="a mock reads its configuration once, at its entrypoint"
# Allowed: a file directly in <harness>-mock/ (the entrypoint package), not in a subdirectory.
allowed='^[a-z0-9]+-mock/[a-z0-9_]+\.go$'
confine 'os\.(Getenv|LookupEnv|Setenv|Unsetenv|Environ)\(' "$allowed" \
  "reads or writes the process environment: read it once at the entrypoint into the configuration"
exit 0
