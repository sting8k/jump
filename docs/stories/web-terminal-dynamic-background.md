# Web Terminal Dynamic Background

## Status

implemented

## Lane

normal

## Product Contract

The Web UI terminal background should match the terminal palette instead of the Web UI chrome theme. Terminal wrapper surfaces such as padding, empty grid area, and scrollable overflow use the configured terminal `theme.background` as their fallback. When terminal output changes the xterm background at runtime with OSC 11, those wrapper surfaces mirror that dynamic background immediately. OSC 111 restores the wrapper fallback to the configured terminal background.

This behavior is browser-local. It does not change PTY output, daemon APIs, relay protocol, session persistence, or Web UI chrome theme switching.

## Relevant Product Docs

- `docs/ARCHITECTURE.md` — browser app owns browser protocol/API contracts and must not couple to daemon internals.
- `docs/stories/webui-theme-preferences.md` — Web UI chrome themes must not mutate `settings.jsonc` / `theme.jsonc` terminal palette behavior.

## Acceptance Criteria

- Terminal shell/container background uses the resolved terminal `theme.background` fallback, including user-provided `~/.config/jump/theme.jsonc` backgrounds.
- OSC 11 runtime background changes from terminal applications are mirrored into the terminal shell/container CSS background without page refresh.
- OSC 111 runtime background restore returns the terminal shell/container CSS background to the resolved terminal fallback.
- Unknown, report-only, or invalid OSC color payloads do not change the wrapper background.
- Switching sessions resets the wrapper background to the resolved terminal fallback before replay/live output can apply that session's OSC colors.
- New browser attaches receive the runner's latest pre-existing OSC 11 background state in the reconnect snapshot, so colors emitted before Web UI attach do not require a manual refresh or new live output.

## Design Notes

- `apps/jump-web/src/terminal.tsx` keeps xterm as the parser/source of truth and registers fall-through OSC handlers for 11 and 111. Returning `false` preserves xterm's built-in dynamic color handling.
- `apps/jump-web/src/terminal-colors.ts` only normalizes supported OSC color payloads into CSS hex colors for wrapper surfaces.
- The browser fix is client-side because `pi-droid-styling` emits OSC 11/111 through PTY output and jumpd already transports that output.
- The runner tracks the latest OSC 11/111 background state and includes it in reconnect snapshots before rendered screen content. This keeps initial attach/reconnect behavior aligned with live WebSocket output without making jumpd interpret terminal escape sequences.

## Validation

| Layer | Expected proof |
| --- | --- |
| Unit | Browser OSC color parsing, terminal background fallback tests, and runner OSC 11/111 snapshot replay tests. |
| Integration | Not required; daemon/API contracts are unchanged. |
| E2E | Not required for this slice; xterm consumes the same replay/live PTY output path. |
| Platform | Web lint/build smoke. |

## Harness Delta

None.

## Evidence

- `corepack pnpm --filter @jump/web test -- terminal-colors.test.ts` passed; Vitest ran the full `@jump/web` suite: 26 files, 409 tests.
- `TMPDIR=/tmp GOWORK=$PWD/go.work go test -v ./cli/jump/internal/ptyserver -run 'TestTerminalColorTracker|TestPTYServerReconnectSnapshotReplaysTerminalBackground'` passed.
- `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./cli/jump/internal/ptyserver ./services/jumpd/internal/wsproxy ./services/jumpd/cmd/jumpd` passed.
- `corepack pnpm --filter @jump/web lint` passed.
- `corepack pnpm --filter @jump/web build` passed.
- `./scripts/build.sh && install -m 755 bin/jump bin/jumpd bin/jump-relayd "$HOME/.local/bin/"` completed and `jumpd status` reported `jumpd 1.15.0 (ready)`.
- Manual Chrome headless attach to a fresh session that emitted OSC 11 before attach (`sess-4bca95d9`) reported `.terminal-shell --terminal-bg` as `#123456` without any live injection.
