# Remote Access Modes

## Status

in_progress

## Lane

normal

## Product Contract

jump has one local baseline and two supported remote-access modes: `tsnet` and
`relay`. `jumpd` owns sessions and user-facing behavior. Access modes transport
traffic to the same handler. The local baseline defaults to localhost but can be
bound to private/VPN/container interfaces with `listen` or `JUMPD_LISTEN` without
becoming a third remote mode. `jump-relayd` is a public transport relay, not a
session store or product-domain service.

## Relevant Product Docs

- `docs/product/remote-access.md`
- `docs/decisions/0004-remote-access-modes.md`

## Acceptance Criteria

- Remote-access docs distinguish runtime core, access mode, and provisioning.
- Docs name exactly two remote-access modes: `tsnet` and `relay`.
- Docs define `jumpd` as the owner of session state and `jump-relayd` as a dumb
  transport component.
- Official release archives include the transport component binary (`jump-relayd`)
  alongside `jump` and `jumpd`.
- Host config supports `listen = "0.0.0.0"` for operators who need the local TCP
  UI reachable from LAN/VPN/container networks, while `JUMPD_LISTEN` remains an
  environment override for deployment systems.
- Docs describe the target optional `[remote].mode` selector without claiming
  that code has already migrated.
- Stale quick-deploy script references are removed from the root README.

## Design Notes

- Commands: target command surface is direct top-level mode commands: `jumpd
  tsnet`, `jumpd relay`, `jumpd status`, and `jumpd doctor`; `jumpd help` should
  surface the local bind option.
- Queries: status should include mode, local URL, remote/public URL, connection
  state, and last actionable error.
- API: no API change selected in this story.
- Tables: no data model change.
- Domain rules: only `jumpd` owns session/workspace domain state. Local TCP bind
  widening remains token-authenticated and is separate from the `tsnet`/`relay`
  remote mode selector.
- UI surfaces: browser UI reaches the same `jumpd` handler through local, tsnet,
  or relay transport.

## Validation

| Layer | Expected proof |
| --- | --- |
| Unit | Config parser tests cover optional `[remote].mode`, `remote.public_url`, `tailscale.auth_key`, and `listen` handling plus env override precedence. |
| Integration | Future `jumpd` startup/config tests for local-only baseline, tsnet, and relay. |
| E2E | Future browser session attach through local and remote URLs. |
| Platform | Future smoke checks for Tailscale/tsnet and relay deployment paths. |
| Release | GoReleaser snapshot/release artifacts include `jump`, `jumpd`, and `jump-relayd`; release docs mention the same. |

## Harness Delta

No harness process change. This story records product/architecture scope for
future implementation work.

## Evidence

- `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./services/jumpd/internal/config ./services/jumpd/cmd/jumpd ./services/jump-relayd/cmd/jump-relayd` passed on 2026-05-21 after adding host `listen` config, help hints, and relayd release packaging coverage.
- `TMPDIR=/tmp GOWORK=$PWD/go.work go vet ./cli/jump/... ./packages/adapter/... ./services/jumpd/... ./services/jump-relayd/...` passed on 2026-05-21 after including relayd in CI vet coverage.
- `./scripts/build.sh` passed on 2026-05-21 and produced `bin/jump`, `bin/jumpd`, and `bin/jump-relayd`.
- A temp-state daemon smoke test passed on 2026-05-21 with `host.toml` containing `listen = "0.0.0.0"` and `port = 18890`; `jumpd status` reported `tcp:    0.0.0.0:18890`.
- `GOWORK=$PWD/go.work go run github.com/goreleaser/goreleaser/v2@latest check` passed on 2026-05-21.
- `GOWORK=$PWD/go.work go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean` passed on 2026-05-21; `dist/jump_1.10.1-snapshot-5f8edb5_linux_amd64.tar.gz` contains `jump`, `jumpd`, and `jump-relayd`.
- `pnpm --filter @jump/website build` and `git diff --check` passed on 2026-05-21 after docs/test-matrix updates.

- `git diff --check` passed on 2026-05-15.
- `pnpm -s build` was attempted on 2026-05-15 and failed before completing
  because the website Astro build requires Node.js `>=22.12.0`; the current
  environment reports Node.js `v20.19.4`.
- `go test ./services/jumpd/internal/config` passed on 2026-05-15 after adding
  optional `[remote].mode` parser coverage.
- `go test ./services/jumpd/cmd/jumpd -run 'TestEnableTailscaleConfig|TestRemoteSetup|TestRunTsnetRelayConfigured|TestRunRelayConfigured|TestDisplayStatus'`
  passed on 2026-05-15.
- `go test ./services/jumpd/cmd/jumpd -run 'TestUsageIncludesNewCommands|TestRunNoArgsPrintsHelp|TestRunHelpCommand|TestRunUnknownCommand|TestEnableTailscaleConfig|TestRemoteSetup|TestRunTsnetRelayConfigured|TestRunRelayConfigured|TestDisplayStatus'`
  passed on 2026-05-15 after switching the command design to direct `jumpd
  tsnet` / `jumpd relay` commands.
- `go test ./services/jumpd/internal/config` and `go test
  ./services/jumpd/internal/tsauth` passed on 2026-05-15 after adding parser
  support for `remote.public_url` and `tailscale.auth_key`, plus passing the
  auth key into tsnet.
- `go test ./services/jumpd/cmd/jumpd -run 'TestRunRelayConfigured|TestRunTsnetRelayConfigured|TestRemoteSetup|TestDisplayStatus'`
  passed on 2026-05-15 after showing `remote.public_url` in `jumpd relay`.
- `go test ./services/jumpd/cmd/jumpd -run 'TestEnableRelayConfig|TestRelaySetup|TestRunRelayConfigured|TestRunTsnetRelayConfigured|TestRemoteSetup|TestDisplayStatus'`
  passed on 2026-05-15 after adding the `jumpd relay` setup/config writer
  flow.
- `go test ./services/jumpd/cmd/jumpd -run 'TestRunDoctor|TestRelayHealthURL|TestUsageIncludesNewCommands'`
  passed on 2026-05-15 after adding `jumpd doctor` diagnostics for config,
  daemon/local UI, tsnet, and relay health.
- `go test ./packages/relayproto` passed on 2026-05-15 after switching the relay
  agent protocol from JSON/text frames to binary frames.
- `go test ./services/jumpd/internal/relayclient` and `go test
  ./services/jump-relayd/cmd/jump-relayd` passed on 2026-05-15 after updating
  both sides to read/write WebSocket binary relay frames.
- `go test ./services/jumpd/cmd/jumpd` passed on 2026-05-15 after shortening
  Unix socket temp paths in the status/auth test helpers.
