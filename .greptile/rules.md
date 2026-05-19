# Greptile Review Rules

- Focus on correctness, regressions, validation gaps, release safety, and maintainability. Avoid low-signal style nits unless they hide a real bug.
- Respect the Harness v0 workflow in `AGENTS.md`: keep product docs, story packets, and `docs/TEST_MATRIX.md` current when behavior or validation expectations change.
- For Web UI changes, check both the Preact state flow and the `jumpd` API contract. Prefer shared pure helpers for logic used by multiple UI surfaces.
- For daemon/CLI changes, check runtime state paths, session lifecycle, and compatibility with existing local state under `~/.local/state/jump/`.
- For build/release changes, check version stamping across Vite `__JUMP_VERSION__`, Go `main.version`, GoReleaser, and local `scripts/build.sh` behavior.
- Treat generated or embedded assets as outputs. Review the source change first and only flag generated files when they are stale or inconsistent.
