mkdir -p .claude/agents
cat > .claude/agents/bgdef.md <<'EOF'
---
name: bgdef
description: Replies with a single word. Use when asked to use bgdef.
background: true
---
Reply with the single word BGDEFREPLY and nothing else.
EOF
