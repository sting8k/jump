/**
 * Frontend configuration: fetch, parse, and resolve.
 *
 * This is the entry point for consumer code. It fetches the raw config from
 * jumpd, delegates to settings-schema.ts for validation and keybinds.ts for
 * keybind resolution, and re-exports everything consumers need.
 *
 * Config files in ~/.config/jump/:
 *   - host.toml       — jumpd behavior (port, network, tailscale)
 *   - settings.jsonc   — frontend preferences (terminal options, keybinds, UI prefs)
 *   - theme.jsonc      — terminal color palette (drop-in Windows Terminal theme compat)
 */

// Re-export schema types and functions that consumers need.
export {
  type SettingsConfig,
  type ThemeColors,
  type Keybind,
  DEFAULT_THEME_COLORS,
  buildTerminalOptions,
  normalizeThemeColors,
} from './settings-schema'

export {
  type ResolvedKeybind,
  IS_MAC,
  DEFAULT_KEYBINDS,
  resolveKeybinds,
  parseKeyCombo,
  keyComboToSequence,
  eventMatchesKeybind,
} from './keybinds'

// ── Fetching ──

import type { SettingsConfig, ThemeColors } from './settings-schema'
import { normalizeAppearance, serializeAppearance, type AppearancePreferences } from './appearance'

export interface FrontendConfig {
  settings: SettingsConfig | null
  themeColors: ThemeColors | null
  appearance: AppearancePreferences | null
}

/**
 * Fetch frontend config from the backend.
 * Returns nulls for missing files (the caller merges with defaults).
 */
export async function fetchFrontendConfig(): Promise<FrontendConfig> {
  try {
    const resp = await fetch('/v1/frontend-config')
    if (!resp.ok) return { settings: null, themeColors: null, appearance: null }
    const json = await resp.json()
    const data = json.data ?? {}
    return {
      settings: data.settings ?? null,
      themeColors: data.theme ?? null,
      appearance: data.appearance == null ? null : normalizeAppearance(data.appearance),
    }
  } catch {
    return { settings: null, themeColors: null, appearance: null }
  }
}

export async function saveFrontendPreferences(appearance: AppearancePreferences, signal?: AbortSignal): Promise<void> {
  const resp = await fetch('/v1/frontend-preferences', {
    method: 'PATCH',
    signal,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ appearance: serializeAppearance(appearance) }),
  })
  if (!resp.ok) {
    throw new Error(`failed to save frontend preferences: ${resp.status}`)
  }
}
