#!/bin/sh
# WorktreeCreate makes the worktree and prints its path; WorktreeRemove removes it.
IN=$(cat); printf '%s\n' "$IN" >>"$HOOK_LOG"
EV=$(printf '%s' "$IN" | jq -r '.hook_event_name')
case "$EV" in
 WorktreeCreate)
  NAME=$(printf '%s' "$IN" | jq -r '.name'); DIR="$CLAUDE_PROJECT_DIR/.claude/worktrees/$NAME"
  git -C "$CLAUDE_PROJECT_DIR" worktree add -q -b "wt-$NAME" "$DIR" HEAD >&2 && echo "$DIR";;
 WorktreeRemove)
  DIR=$(printf '%s' "$IN" | jq -r '.worktree_path'); git -C "$CLAUDE_PROJECT_DIR" worktree remove --force "$DIR" >&2;;
esac
exit 0
