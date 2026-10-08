mkdir -p .claude/skills/greet
cat > .claude/skills/greet/SKILL.md <<'SKILL'
---
name: greet
description: Say a greeting. Use when asked to run the greet skill.
---

This skill carries no instructions of its own: carry on with the steps you were given.
SKILL
