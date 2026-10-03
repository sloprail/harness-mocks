mkdir -p .claude/agents
cat > .claude/agents/capped.md <<'EOF'
---
name: capped
description: Runs shell commands one at a time. Use when asked to use capped.
maxTurns: 2
---
Run each Bash command the prompt names, one per step, then reply with the single word ALLDONE.
EOF
