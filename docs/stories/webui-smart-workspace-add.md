# WebUI Smart Workspace Add

## Status

implemented

## Lane

normal

## Product Contract

The Home page and Manage Projects add-workspace inputs keep their existing visual layout while resolving typed workspace text more intelligently. Absolute/path-like input continues to add that path. Non-path input fuzzy-matches recent session workspace roots and discovered projects so short queries can resolve to a workspace directory without opening a new picker. When suggestions are visible, pressing Tab completes the input to the first suggestion.

## Relevant Product Docs

- `README.md` (Web UI feature surface)

## Acceptance Criteria

- Home `Add workspace dir` input accepts existing path-like values unchanged.
- Home input accepts non-path fuzzy queries when a recent or discovered workspace suggestion exists, and Add/Enter adds the top suggestion.
- Manage Projects Smart add uses the same suggestion ranking as Home.
- Filesystem completions stay preferred for path-like queries.
- Already configured workspace paths are filtered from recent/discovered suggestions and still show duplicate state for direct path input.
- Tab completes the input to the first visible suggestion in Home and Manage Projects.
- No new modal or visual layout is introduced.

## Design Notes

- Shared suggestion logic lives in `apps/jump-web/src/workspace-suggestions.ts` so Home and Manage Projects do not drift.
- Ranking is local frontend behavior based on filesystem completions, recent sessions, discovered projects, active counts, session counts, and fuzzy token matches.
- API contracts are unchanged; existing `/v1/projects/add` and `/v1/fs/complete` endpoints are reused.

## Validation

| Layer | Expected proof |
| --- | --- |
| Unit | Workspace suggestion ranking/filtering tests. |
| Integration | Existing frontend typecheck. |
| E2E | Not required; no route/API shape change. |
| Platform | Not required; browser UI behavior only. |
| Release | Not required. |

## Harness Delta

None.

## Evidence

- `pnpm --filter @jump/web test -- workspace-suggestions.test.ts` passed (Vitest ran 21 files, 352 tests).
- `pnpm --filter @jump/web lint` passed.
- `git diff --check` passed.
