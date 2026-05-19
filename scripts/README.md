# Scripts

This directory contains project automation used by CI, E2E tests, and local development.

## Current Scripts

- `build.sh` builds the protocol package, builds the web app, syncs `apps/jump-web/dist` into the embedded `jumpd` web directory, then writes `bin/jump` and `bin/jumpd`. It stamps binaries and web assets from the latest `v*` git tag by default, falling back to the root `package.json` version when tags are unavailable; set `VERSION` to override it.
- `dev-kill.sh` is a best-effort pre-dev cleanup that stops an existing `jumpd` daemon via `bin/jumpd` or `jumpd` on `PATH`; it does not kill arbitrary processes.

## Installer

The upstream installer applies the Harness v0 operating files and folder
structure to a target project directory. It defaults to the current directory,
accepts a target path, and asks interactive users whether to `1. Merge`,
`2. Override`, or `3. Stop` when the target already contains `AGENTS.md`,
`docs/`, or `scripts/`.
Non-interactive installs stop on those protected paths unless `--merge` or
`--override` is provided.

```bash
curl -fsSL "https://raw.githubusercontent.com/hoangnb24/harness-experimental/main/scripts/install-harness.sh?$(date +%s)" | bash -s -- --yes
```

```bash
curl -fsSL "https://raw.githubusercontent.com/hoangnb24/harness-experimental/main/scripts/install-harness.sh?$(date +%s)" | bash -s -- --merge --yes
```

The installer must stay limited to harness files. Do not use it to scaffold
application source folders, package scripts, CI, tests, platform shells, or fake
validation commands. The installer script is not part of the installed project
payload.

## Future Command Contract

Expected future checks:

```text
validate:quick
  format, lint, typecheck, unit tests, architecture check

test:integration
  backend contract and integration checks

test:e2e
  user-visible end-to-end flows

test:platform
  platform shell smoke checks, if the project has a native shell

test:release
  full suite, log checks, and performance smoke
```
