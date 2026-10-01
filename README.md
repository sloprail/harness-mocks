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

## Building and testing claude-mock

```
cd claude-mock
make build      # builds ../bin/a10n-claude-mock
make test-unit  # unit tests
make test-e2e   # full e2e suite, driving the built binary
```

## Building and testing cursor-mock

`cursor-mock/` is the same kind of stand-in for the `cursor-agent` CLI (print
mode, `--output-format stream-json`): a scenario script plays the agent, the
mock fires Cursor's hooks from `.cursor/hooks.json` and runs the Shell, Read and
Write tools. It is an adapter on the shared core in `internal/`. Its e2e suite
replays the runs recorded in `cursor-mock/snapshots/` (captured with
`snapshots/capture.sh` against the installed `cursor-agent`; needs `jq`).

```
cd cursor-mock
make build      # builds ../bin/a10n-cursor-mock
make test-unit  # unit tests
make test-e2e   # e2e suite, driving the built binary
```

## Releases

Pushing a tag matching `v*` (or running the `publish claude-mock` workflow
manually) builds `linux/amd64` and `linux/arm64` binaries and publishes them
as assets on a GitHub Release — see
[`.github/workflows/publish-claude-mock.yml`](.github/workflows/publish-claude-mock.yml).
The release build runs the full test suite first.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Every external pull request needs a
signed [CLA](CLA.md), and commits must be signed (GPG/SSH) — GitHub's commit
verification badge shows this on the PR.

## License

[MIT](LICENSE)
