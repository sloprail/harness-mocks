#!/bin/sh
# Trusts the scratch repository as a project, so that codex lists (and loads) its project layer.
set -e
printf '[projects."%s"]\ntrust_level = "trusted"\n' "$(pwd -P)" >>"$CODEX_HOME/config.toml"
