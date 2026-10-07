#!/bin/sh
# Trusts hooks in the run's config.toml by the key and hash Codex gives them: the user layer's hooks.json
# under CODEX_HOME, the project's under the repository (both canonical paths).
set -e
home=$(cd "$CODEX_HOME" && pwd -P)
repo=$(pwd -P)
: >>"$CODEX_HOME/config.toml"
printf '[hooks.state."%s/.codex/hooks.json:pre_tool_use:0:0"]\ntrusted_hash = "sha256:fe1d1399af4be296a812de676629165e585a6b201058fb71959eb1bd91cc5d4d"\n\n' "$repo" >>"$CODEX_HOME/config.toml"
