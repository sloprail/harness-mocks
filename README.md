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

## Snapshots: harness versions and doc pages

Each `<harness>-mock/snapshots/` holds recordings of the real harness
(`runs/<name>/`) and the hashes of the doc pages the capabilities cite
(`MANIFEST.yaml`). Two versions are kept apart:

- The binary a recording was made with is the run's own: `version` in its
  `run.yaml`. The MANIFEST's `pin` is only which binary `capture.sh` runs next.
- A doc page is frozen by its own `sha256` (and the `fetched` date). Only a
  page whose `sha256` changed puts the capabilities that cite it back in
  judgement; bumping `pin`, or re-freezing a page that hashes the same,
  invalidates nothing. See [adr/pinned-harness-versions](adr/pinned-harness-versions/ADR.md).

`capture.sh` never uses the `claude`/`codex`/`cursor-agent` on your `PATH`: it
runs the pinned version from a cache of its own, so a global auto-update does not
change what is recorded.

```
snapshots/capture.sh pin <version>            # install that exact binary, make it the pin
snapshots/capture.sh run <name> [--rerecord]  # a run recorded at another version needs --rerecord
go run ./tools/harness-bin install claude 2.1.285   # or: path (prints the binary, checks --version)
```

`tools/harness-bin install|path <claude|codex|cursor> <version>` installs into
`$HARNESS_BIN_CACHE` (default: the per-user cache dir, shared by worktrees), never
the global install, and `path` fails unless the binary reports exactly that version.

| harness | installed from | auto-update |
| --- | --- | --- |
| claude | npm `@anthropic-ai/claude-code@<v>` | `DISABLE_AUTOUPDATER=1` |
| codex | npm `@openai/codex@<v>` | no switch; npm installs are updated by npm only |
| cursor | the versioned download `downloads.cursor.com/lab/<date>-<build>/<os>/<arch>/agent-cli-package.tar.gz` | `--disable-auto-update` (hidden flag) |

cursor-agent publishes every build at a URL that carries its commit hash
(`2026.09.28-64d2043`), but only the current build's hash is published (in
`https://cursor.com/install`, which installs the latest one only). `harness-bin`
keeps a table of the dates it has been pinned at (`cursorBuilds`) and also accepts
the full `<date>-<hash>`; an older date without a known hash is refused.

Authentication is yours, not the cache's: the fake `HOME` of a capture links your
Keychains (claude, cursor-agent) or `~/.codex/auth.json` (codex), and nothing is copied.

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
