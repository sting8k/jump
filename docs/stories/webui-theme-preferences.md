# Web UI Theme Preferences

## Status

implemented

## Lane

normal

## Product Contract

The Web UI has client-side theme switching for chrome/UI surfaces. It ships the existing `default` theme, a `spacetime` theme inspired by SpacetimeDB's dark blue-black surfaces, a `vercel` theme with a Command Center direction using black cockpit surfaces, HUD-cyan state signals, tactical grid texture, selected-session lock-on treatment, and instrument-style telemetry, an `atelier` theme with a Silver Atelier direction using warm graphite surfaces, muted silver-gold accents, softer material depth, and calmer daily-use spacing, plus a `hud` theme with a Signal HUD direction using blue-gray instrument glass, muted green/teal status accents, subtle grid texture, and angular telemetry frames. The selected `theme_id` is applied immediately in the browser, cached locally for first paint, and persisted by `jumpd` as server-managed Web UI state without page reloads or terminal/session reconnects.

## Relevant Product Docs

- No dedicated product doc yet; this story is the current contract for the theme preference foundation.

## Acceptance Criteria

- The shipped themes are `default`, `spacetime`, `vercel`, `atelier`, and `hud`; `default` keeps the existing Web UI visuals.
- The browser applies cached appearance before app mount to avoid a first-paint theme flash.
- The top-right `...` app menu renders theme options as a compact one-row swatch button group with tooltips when multiple themes exist and switches the Web UI instantly.
- `GET /v1/frontend-config` returns the server-managed appearance preference alongside existing frontend config.
- `PATCH /v1/frontend-preferences` validates and atomically persists `appearance.theme_id` under jumpd state.
- Unknown or unsafe theme ids fall back to `default` on the client and are rejected by the server write path.
- The slice does not change terminal/xterm color palette behavior or mutate `settings.jsonc` / `theme.jsonc`.

## Design Notes

- Commands: no CLI behavior change.
- Queries: extend `/v1/frontend-config` with `appearance`.
- API: add `PATCH /v1/frontend-preferences` with `{ "appearance": { "theme_id": "vercel" } }` or any other whitelisted theme id such as `atelier` or `hud`.
- Tables: no database; server state is `~/.local/state/jump/web-preferences.json`.
- Domain rules: theme ids are whitelisted by client and server; future themes extend the catalog, server whitelist, and one theme CSS file under `apps/jump-web/src/themes/`.
- UI surfaces: the top-right `...` app menu hosts compact theme swatches for the Web UI chrome only.

## Validation

| Layer | Expected proof |
| --- | --- |
| Unit | Web appearance parser/cache tests; jumpd webprefs load/save/validation tests. |
| Integration | jumpd frontend config/preferences route tests. |
| E2E | Not required for token-only theme switching in this slice. |
| Platform | Web lint/build smoke and Go command package tests. |
| Release | Normal CI checks. |

## Harness Delta

None.

## Evidence

- `TMPDIR=/tmp GOWORK=$PWD/go.work go test ./services/jumpd/internal/webprefs ./services/jumpd/cmd/jumpd` passed.
- `pnpm --filter @jump/web test -- appearance.test.ts store.test.ts --runInBand` passed (Vitest ran the full `@jump/web` suite: 22 files, 366 tests).
- `pnpm --filter @jump/web lint` passed.
- `pnpm --filter @jump/web build` passed.
- `git diff --check` passed.
