#!/usr/bin/env bash
set -uo pipefail
payload="$(cat)"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/changeset.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/adr.sh"
. "${SR_GUARDRAIL_DIR:-.}/../../_lib/confine.sh"
ADR_TITLE="only internal/procexec starts a child process"
confine 'exec\.Command(Context)?\(|\.Env[[:space:]]*=[^=]' '^internal/procexec/' \
  "starts a process or sets its environment: use internal/procexec"
exit 0
