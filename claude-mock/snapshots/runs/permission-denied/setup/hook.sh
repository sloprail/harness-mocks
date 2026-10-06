#!/bin/sh
IN=$(cat); printf "%s\n" "$IN" >>"$HOOK_LOG"
