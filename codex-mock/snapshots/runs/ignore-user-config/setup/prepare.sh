#!/bin/sh
# Trusts hooks in the run's config.toml by the key and hash Codex gives them: the user layer's hooks.json
# under CODEX_HOME, the project's under the repository (both canonical paths).
set -e
home=$(cd "$CODEX_HOME" && pwd -P)
repo=$(pwd -P)
: >>"$CODEX_HOME/config.toml"
printf '[hooks.state."%s/hooks.json:pre_tool_use:0:0"]\ntrusted_hash = "sha256:d978dfb3851e6eb6af88fd8323e578998edac8eb593f6b962c2ede394cbe6b41"\n\n' "$home" >>"$CODEX_HOME/config.toml"
