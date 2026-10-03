#!/bin/sh
# Two earlier sessions of another project already in the (hermetic) home, each
# with a sub-agent transcript beside it: one last written in August, the other
# just now. The run that follows starts a new session; what is left of the two
# afterwards is what the retention sweep (cleanupPeriodDays, 30 days by
# default) removes and what it leaves.
p="$HOME/.claude/projects/-earlier-project"
for s in oldsess recentsess; do
  mkdir -p "$p/$s/subagents"
  printf '%s\n' '{"type":"user","message":{"role":"user","content":"earlier"},"uuid":"11111111-1111-4111-8111-111111111111","parentUuid":null,"sessionId":"'"$s"'"}' >"$p/$s.jsonl"
  printf '%s\n' '{"type":"user","message":{"role":"user","content":"sub"},"uuid":"22222222-2222-4222-8222-222222222222","parentUuid":null,"isSidechain":true,"agentId":"aaaa","sessionId":"'"$s"'"}' >"$p/$s/subagents/agent-aaaa.jsonl"
  printf '%s\n' '{"agentType":"general-purpose","description":"d","toolUseId":"toolu_x","spawnDepth":1}' >"$p/$s/subagents/agent-aaaa.meta.json"
done
touch -t 202608010000 "$p/oldsess.jsonl" "$p/oldsess/subagents/agent-aaaa.jsonl" "$p/oldsess/subagents/agent-aaaa.meta.json" "$p/oldsess/subagents" "$p/oldsess"
mkdir -p "$HOME/.claude" && printf %s "{\"cleanupPeriodDays\": 30}" > "$HOME/.claude/settings.json"
