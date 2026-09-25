# harness-mocks

A monorepo of harness/agent-CLI emulators used for evals.

Each emulator lives in its own top-level directory (e.g. `claude-mock/`) so
this repo can grow to hold mocks for multiple agent harnesses over time.
Claude Code's mock (`claude-mock/`) is the first: a deterministic
replacement for the `claude` binary used to drive e2e tests without calling
a real model.

`claude-mock/` was extracted from `a10n-cli`'s `services/claude-mock` via
[`git-filter-repo`](https://github.com/newren/git-filter-repo), preserving
its full commit history.
