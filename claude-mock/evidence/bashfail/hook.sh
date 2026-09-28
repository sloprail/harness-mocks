#!/bin/sh
IN=$(cat); printf '%s\n' "$IN" >> "$(dirname "$0")/payloads.jsonl"
