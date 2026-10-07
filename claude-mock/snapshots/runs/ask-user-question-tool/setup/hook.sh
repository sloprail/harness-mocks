#!/bin/sh
# Logs every hook payload, and answers an AskUserQuestion the documented way for a run with no terminal
# (hooks#allow-with-updatedinput): allow it, with updatedInput holding the questions and their answers.
# A question whose text starts with "Custom" gets an answer that is no option's label, the rest the second
# option's label (a multiple-choice one, its first two, joined by a comma and a space).
IN=$(cat); printf '%s\n' "$IN" >>"$HOOK_LOG"
if [ "$(printf '%s' "$IN" | jq -r '[.hook_event_name, .tool_name] | join(" ")')" = "PreToolUse AskUserQuestion" ]; then
  printf '%s' "$IN" | jq -c '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "allow",
    updatedInput: (.tool_input + {answers: (.tool_input.questions | map({(.question): (
      if (.question | startswith("Custom")) then "something else entirely"
      elif .multiSelect then (.options | map(.label) | .[0:2] | join(", "))
      else .options[1].label end)}) | add)})}}'
fi
