# Web UI Theme Preferences

## Status

implemented

## Lane

normal

## Product Contract

The Web UI has client-side theme switching for chrome/UI surfaces. It ships the existing `default` theme plus a `spacetime` theme inspired by SpacetimeDB's dark blue-black surfaces, thin cold borders, grid/radial background fields, glassier panels, and neon green/blue/purple accents. The selected `theme_id` is applied immediately in the browser, cached locally for first paint, and persisted by `jumpd` as server-managed Web UI state without page reloads or terminal/session reconnects.

## Relevant Product Docs

- No dedicated product doc yet; this story is the current contract for the theme preference foundation.

## Acceptance Criteria

- The shipped themes are `default` and `spacetime`; `default` keeps the existing Web UI visuals.
- The browser applies cached appearance before app mount to avoid a first-paint theme flash.
- The top-right `...` app menu renders theme options as a compact one-row swatch button group with tooltips when multiple themes exist and switches the Web UI instantly.
- `GET /v1/frontend-config` returns the server-managed appearance preference alongside existing frontend config.
- `PATCH /v1/frontend-preferences` validates and atomically persists `appearance.theme_id` under jumpd state.
- Unknown or unsafe theme ids fall back to `default` on the client and are rejected by the server write path.
- The slice does not change terminal/xterm color palette behavior or mutate `settings.jsonc` / `theme.jsonc`.

## Design Notes

- Commands: no CLI behavior change.
- Queries: extend `/v1/frontend-config` with `appearance`.
- API: add `PATCH /v1/frontend-preferences` with `{ "appearance": { "theme_id": "spacetime" } }`.
- Tables: no database; server state is `~/.local/state/jump/web-preferences.json`.
- Domain rules: theme ids are whitelisted by client and server; future themes extend the catalog, server whitelist, and CSS token blocks.
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
- `pnpm --filter @jump/web test -- appearance.test.ts store.test.ts --runInBand` passed (Vitest ran the full `@jump/web` suite: 22 files, 362 tests).
- `pnpm --filter @jump/web lint` passed.
- `pnpm --filter @jump/web build` passed.
- `git diff --check` passed.
