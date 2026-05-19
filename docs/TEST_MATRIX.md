# Test Matrix

This file maps product behavior to proof.

No product behavior has been defined or implemented yet. Do not mark a row
implemented until tests or validation evidence exist.

## Status Values

| Status | Meaning |
| --- | --- |
| planned | Accepted as intended behavior, not implemented |
| in_progress | Actively being built |
| implemented | Implemented and proof exists |
| changed | Contract changed after earlier implementation |
| retired | No longer part of the product contract |

## Matrix

| Story | Contract | Unit | Integration | E2E | Platform | Status | Evidence |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `docs/stories/remote-access-modes.md` | Local baseline plus `tsnet` and `relay` remote-access modes | yes | no | no | no | in_progress | `go test ./packages/relayproto`; `go test ./services/jumpd/internal/config`; `go test ./services/jumpd/internal/tsauth`; `go test ./services/jumpd/internal/relayclient`; `go test ./services/jump-relayd/cmd/jump-relayd`; `go test ./services/jumpd/cmd/jumpd` |
| `docs/stories/mobile-resume-reconnect.md` | Mobile browser resume should reconnect stale UI transports without manual refresh | yes | no | no | no | implemented | `pnpm --filter @jump/web test`; `pnpm --filter @jump/web lint`; `pnpm --filter @jump/web build`; `go test ./services/jumpd/cmd/jumpd` |
| `docs/stories/web-terminal-font-size.md` | Web UI should let users adjust terminal font size without daemon config/restart | yes | no | no | no | implemented | `pnpm --filter @jump/web test -- terminal-font-size page-resume`; `pnpm --filter @jump/web lint`; `pnpm --filter @jump/web build`; `go test ./services/jumpd/cmd/jumpd` |
| `docs/stories/webui-terminal-pasture-skin.md` | Runtime Web UI keeps existing flows while using the Terminal Pasture palette plus sharp CodeUI-inspired micro-polish, inline action icons, crisp framing, real host telemetry with optional battery plus header PTY alive/dead counts, and add-workspace suggestion focus retention | yes | yes | no | yes | implemented | `pnpm --filter @jump/web test`; `pnpm --filter @jump/web lint`; `pnpm --filter @jump/web build`; `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./services/jumpd/internal/hostmetrics ./services/jumpd/cmd/jumpd`; local rebuilt `jumpd status` on `127.0.0.1:8790`; embedded CSS/API token/no-CRT/no-blur host-telemetry checks; `git diff --check` |
| `docs/stories/webui-smart-workspace-add.md` | Web UI add-workspace inputs keep the same layout while supporting fuzzy recent/discovered workspace selection and Tab completion to the first visible suggestion | yes | no | no | no | implemented | `pnpm --filter @jump/web test -- workspace-suggestions.test.ts`; `pnpm --filter @jump/web lint`; `./scripts/build.sh`; local `jumpd 1.7.1 (ready)` health check; `git diff --check` |
| `docs/stories/session-latency-retention/` | Small PTY output flushes faster while local dead sessions are automatically pruned after 24 hours or when exit timestamps are invalid | yes | yes | no | yes | implemented | `TMPDIR=/tmp go test ./cli/jump/internal/ptyserver ./services/jumpd/internal/sessionfiles ./services/jumpd/cmd/jumpd`; `TMPDIR=/tmp go build -o /tmp/jump-verify/jump ./cli/jump/cmd/jump`; `TMPDIR=/tmp go build -o /tmp/jump-verify/jumpd ./services/jumpd/cmd/jumpd`; local rebuilt-runner latency probe |
| `docs/stories/host-display-sleep-action.md` | Web UI exposes a guarded host display sleep action in the top-right menu, available only on macOS hosts with executable `pmset`, disabled otherwise, and showing best-effort `awake`/`asleep`/`unknown` state | yes | yes | no | yes | implemented | `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./services/jumpd/internal/hostactions ./services/jumpd/cmd/jumpd`; `GOOS=darwin GOARCH=arm64 TMPDIR=/tmp GOWORK=$PWD/go.work go test -c ./services/jumpd/internal/hostactions`; `pnpm --filter @jump/web test -- host-actions.test.ts`; `pnpm --filter @jump/web lint` |
| `docs/stories/web-terminal-vim-freeze.md` | Full-screen TUIs such as Vim must not leave the Web UI terminal stuck until refresh when synchronized output escape sequences split across frames or a WS client stalls | yes | yes | no | no | implemented | `pnpm --filter @jump/web test -- terminal-io.test.ts --runInBand`; `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./cli/jump/internal/ptyserver ./services/jumpd/internal/wsproxy ./services/jumpd/cmd/jumpd` |
| `docs/stories/web-release-update-badge.md` | Web UI `...` menu shows an informational GitHub latest-release update row when `jumpd` health reports `update_available`, with no auto-update or session mutation | yes | yes | no | yes | implemented | `pnpm --filter @jump/web test -- release-updates.test.ts store.test.ts --runInBand`; `pnpm --filter @jump/web lint`; `pnpm --filter @jump/web build`; `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./services/jumpd/internal/update`; `git diff --check` |
| `docs/stories/product-rename-jump/` | Product identity is hard-renamed from `gmux` to `jump` across binaries, paths, modules, docs, and runtime defaults with no old-path fallback | yes | yes | no | yes | implemented | `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./packages/paths/... ./packages/workspace/... ./packages/relayproto/... ./packages/scrollback/... ./packages/adapter/... ./cli/jump/... ./services/jump-relayd/... ./services/jumpd/...`; `pnpm --filter @jump/web lint`; `pnpm --filter @jump/web test`; `pnpm --filter @jump/web build`; `TMPDIR=/tmp GOWORK=$PWD/go.work go build -o /tmp/jump-verify/jump ./cli/jump/cmd/jump`; `TMPDIR=/tmp GOWORK=$PWD/go.work go build -o /tmp/jump-verify/jumpd ./services/jumpd/cmd/jumpd`; `TMPDIR=/tmp GOWORK=$PWD/go.work go build -o /tmp/jump-verify/jump-relayd ./services/jump-relayd/cmd/jump-relayd`; `pnpm --filter @jump/protocol build`; `pnpm --filter @jump/protocol test`; `pnpm --filter @jump/protocol lint`; `source "$HOME/.nvm/nvm.sh" && nvm use 23 && pnpm --filter @jump/website build`; workflow script tests; `git diff --check`; old-name/public URL text checks |

## Evidence Rules

- Unit proof covers pure domain and application rules.
- Integration proof covers backend enforcement, data integrity, provider
  behavior, jobs, or service contracts.
- E2E proof covers user-visible browser flows.
- Platform proof covers only shell, deployment, mobile, desktop, or runtime
  behavior that cannot be proven in lower layers.
- A story can be implemented without every proof column if the story packet
  explains why.
