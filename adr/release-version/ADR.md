---
concern: how a mock binary learns its own release version
sloprails: [file-guard/adr-conformance]
---

# A mock's version is stamped at release build time and reported by --version

## Concern

How a released mock binary knows which release it is, so an installer or a user can check what they have.

## Decision

- The release workflow stamps the tag, without its leading `v`, into a package-level `version` string in the mock's entrypoint package (`-ldflags "-X main.version=<tag>"`). An unstamped build reports `dev`.
- `<mock> --version` prints that string and exits 0. The version is read nowhere else: it is not a flag or environment variable, and no other behaviour depends on it.
- A mock does not answer `--version` the way the harness it imitates does: `--version` is the build's version, never the harness's version.
