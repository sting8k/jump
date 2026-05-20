# Web Terminal Mode Replay

## Status

implemented

## Lane

normal with stronger validation

## Problem

Interactive TUIs can enable terminal modes such as mouse tracking, SGR mouse
coordinates, application cursor keys, focus events, and bracketed paste once at
startup. A browser client that attaches after those startup sequences, or
reconnects from a stale page, receives Jump's screen snapshot but not the prior
mode-enable sequences. The visible screen can look correct while clicks or keys
are interpreted by the browser/xterm locally instead of being sent to the TUI.

This was observed with `hunk` CLI: mouse interaction sometimes worked, sometimes
needed a refresh, and sometimes never became interactive.

## Contract

- Reconnect snapshots must restore terminal input modes that affect browser-side
  xterm event encoding.
- Mouse tracking modes enabled by the child process must be replayed before the
  snapshot becomes interactive.
- Disabling a mode must remove it from future snapshot replay.
- Snapshot replay must preserve the existing screen/cursor behavior and avoid
  replaying speculative modes that were not enabled by the child.

## Design Notes

- `ptyserver` now tracks mode enable/disable callbacks from the virtual terminal
  emulator.
- Only input-relevant modes are normalized: cursor-key/keypad modes, mouse
  tracking/encoding modes, focus events, and bracketed paste.
- The reconnect frame first resets that known input-mode set, then re-enables the
  modes currently active in the virtual terminal. This handles both fresh xterm
  instances and stale reconnecting instances whose browser-side mode state may
  no longer match the child process.

## Validation

- Unit tests cover enabling mouse/SGR/bracketed-paste modes, replaying them, and
  removing a disabled mode from replay.
- Existing PTY server tests verify reconnect snapshot, resize, input, and screen
  rendering behavior still pass.

## Evidence

- `TMPDIR=/tmp GOWORK=$PWD/go.work go test -count=1 ./cli/jump/internal/ptyserver` passed three consecutive runs.
- `TMPDIR=/tmp GOWORK=$PWD/go.work go test -count=1 ./cli/jump/internal/ptyserver ./services/jumpd/internal/wsproxy ./services/jumpd/cmd/jumpd` passed.
