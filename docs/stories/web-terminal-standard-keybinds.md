# Web Terminal Standard Keybinds

## Status

implemented

## Lane

normal with stronger validation

## Product Contract

The Web terminal default keymap should include common Linux/Windows terminal edit shortcuts so users can copy, paste, and select all terminal content without custom settings. Existing macOS defaults and `macCommandIsCtrl` behavior remain unchanged.

## Relevant Product Docs

- `docs/ARCHITECTURE.md`
- `apps/website/src/content/docs/using-the-ui.md`
- `apps/website/src/content/docs/reference/settings.md`

## Acceptance Criteria

- Linux/Windows defaults include `Ctrl+Insert` for copy.
- Linux/Windows defaults include `Shift+Insert` for paste.
- Linux/Windows defaults include `Ctrl+Shift+A` for select all terminal content.
- Existing `Ctrl+C`, `Ctrl+Shift+C`, `Ctrl+V`, `Ctrl+Shift+V`, and browser-stolen-key fallbacks remain unchanged.
- macOS default keymap and documented `macCommandIsCtrl` behavior remain unchanged.
- Default keymap documentation lists the new Linux/Windows shortcuts.

## Design Notes

- Commands: none.
- Queries: none.
- API: no daemon/API/protocol change; browser keymap defaults only.
- Tables: none.
- Domain rules: clipboard/select-all actions still go through the explicit keymap rather than browser/xterm passthrough.
- UI surfaces: Web terminal keyboard handling and website keyboard shortcut docs.

## Validation

| Layer | Expected proof |
| --- | --- |
| Unit | Focused `keybinds.test.ts` coverage for the new default mappings. |
| Integration | `@jump/web` lint/build and docs build as part of release verification. |
| E2E | Not required; key resolution is pure browser-side keymap behavior. |
| Platform | Manual browser/terminal smoke recommended after release for Ctrl+Insert/Shift+Insert, especially on non-Mac desktops. |
| Release | Version bump, push, and release workflow. |

## Harness Delta

None.

## Evidence

- `corepack pnpm --filter @jump/web test -- keybinds.test.ts` passed; Vitest ran the full `@jump/web` suite: 26 files, 410 tests.
- `corepack pnpm --filter @jump/web lint` passed.
- `corepack pnpm --filter @jump/web build` passed.
- `corepack pnpm --filter @jump/website build` passed.
- `git diff --check` passed.
- `srcwalk review --scope apps/jump-web/src --limit 8` and `srcwalk review --scope docs --limit 8` reviewed the changed keybind and story/matrix evidence.
