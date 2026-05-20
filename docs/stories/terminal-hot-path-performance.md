# Terminal Hot Path Performance

## Status

implemented

## Lane

normal with stronger validation

## Problem

Terminal output can arrive in bursts from shells, TUIs, and agent tools. The hot
path should keep latency bounded without letting one slow browser client stall
other clients or the local terminal attach.

## Contract

- A slow WebSocket terminal client must not block PTY output delivery to healthy
  terminal clients.
- Reconnect snapshot network writes must not hold the PTY server lock while the
  client is slow.
- Browser terminal output should batch queued chunks before `xterm.write` to
  reduce parser/write-callback churn during bursts.
- Existing synchronized-output scroll preservation, resize ordering, and stale
  epoch dropping must remain correct.
- Performance instrumentation must be available without changing normal UI
  behavior.

## Design Notes

- `ptyserver` now gives each WebSocket client a bounded outbound queue. Live
  output and resize events are enqueued without blocking the PTY flush path; a
  full client queue cancels that client so it can reconnect from a fresh
  snapshot.
- Reconnect snapshots are still built from the virtual terminal under the screen
  lock, but the potentially slow network write happens after client registration
  and after releasing the PTY server lock. Any live output produced during the
  snapshot write waits behind the snapshot in that client's outbound queue.
- `TerminalIO` coalesces queued terminal chunks into a bounded `xterm.write`
  batch while preserving write-callback ordering and BSU/ESU scroll handling.
- Runner performance counters are exposed on the owner-only session socket at
  `GET /debug/perf`. Browser write metrics can be logged by setting
  `localStorage['jump:terminal-perf'] = '1'` or `window.__JUMP_TERMINAL_PERF__ =
  true`.
- Larger changes, such as a dedicated VT snapshot goroutine or async scrollback
  writer, are intentionally deferred until the new metrics show those costs are
  material.

## Validation

- Go unit tests cover the slow-client queue policy and `/debug/perf` endpoint.
- Frontend unit tests cover coalesced writes, write metrics, existing watchdog
  behavior, split terminal sequence handling, resize ordering, and scroll
  preservation.
- TypeScript compile/lint validates the Web UI wiring.

## Evidence

- `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./cli/jump/internal/ptyserver ./services/jumpd/internal/wsproxy ./services/jumpd/cmd/jumpd` passed.
- `pnpm --filter @jump/web test -- terminal-io.test.ts --runInBand` passed
  (Vitest suite ran all 21 Web UI test files).
- `pnpm --filter @jump/web lint` passed.
- `pnpm --filter @jump/web build` passed.
