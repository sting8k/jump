# Web Terminal Mobile IME Input

## Status

implemented

## Lane

normal

## Product Contract

Mobile users typing into the Web terminal with Vietnamese Telex or similar IMEs
must not lose characters, duplicate committed text, or corrupt accents while the
keyboard rewrites its hidden textarea buffer. Manual backspace/edit flows must
keep the terminal line and xterm hidden textarea aligned before subsequent IME
corrections.

## Relevant Product Docs

- `docs/product/session-lifecycle.md`

## Acceptance Criteria

- Mobile IME pre-edit text is not sent to the PTY before the IME commits it.
- Committed IME text is sent exactly once and normalized to NFC.
- Android-style delete-plus-insert word replacements translate into terminal
  backspaces plus inserted text without xterm also sending stale textarea diffs.
- Terminal backspace output keeps xterm's hidden textarea in sync so correcting a
  word after deleting characters does not erase or re-send earlier text.
- The input diagnostics page can focus the xterm textarea on mobile so real
  keyboard traces can be collected.

## Design Notes

- UI surfaces: Web terminal xterm hidden textarea and `/_/input-diagnostics`.
- Domain rules: PTY echo remains the source of truth; browser-side handlers only
  gate or translate mobile keyboard events before bytes cross the WebSocket.
- Mobile correction model: Android Chrome/Gboard often emits `deleteContentBackward`
  followed by `insertText` without composition events; the handler treats this as
  a replacement only when the delete has a non-collapsed selection.

## Validation

| Layer | Expected proof |
| --- | --- |
| Unit | Terminal input gate tests cover pre-edit suppression, duplicate commit suppression, and NFC commits; mobile input tests cover replacement and backspace sync. |
| Integration | Not required; behavior is browser event translation before WebSocket send. |
| E2E | Not automated; real mobile keyboard behavior is captured with `/_/input-diagnostics` and manually verified. |
| Platform | Android Chrome/Gboard Vietnamese Telex manual verification covers typing, deleting back to a prior word, and re-correcting accents. |
| Release | `./scripts/build.sh` passes and local `bin/jumpd` serves the rebuilt web UI. |

## Harness Delta

None.

## Evidence

- `pnpm --filter @jump/web test -- mobile-input.test.ts terminal-input.test.ts` passed on 2026-05-21.
- `pnpm --filter @jump/web lint` passed on 2026-05-21.
- `./scripts/build.sh` passed on 2026-05-21.
- Local `./bin/jumpd status` reported `jumpd 1.10.1 (ready)` on `127.0.0.1:8790` after redeploy from repo `bin/jumpd`.
- Manual Android Chrome/Gboard Vietnamese Telex verification passed on 2026-05-21 for typing, deleting several characters, and re-correcting Vietnamese accents.
