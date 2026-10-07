#!/bin/sh
# Trusts the project (the repository) in the run's config.toml, so its .codex/hooks.json layer loads.
set -e
repo=$(pwd -P)
: >>"$CODEX_HOME/config.toml"
printf '[projects."%s"]\ntrust_level = "trusted"\n' "$repo" >>"$CODEX_HOME/config.toml"
