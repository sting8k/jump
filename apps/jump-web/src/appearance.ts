export interface ThemeDefinition {
  id: string
  label: string
  themeColor: string
}

export interface AppearancePreferences {
  themeId: string
}

interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

export const APPEARANCE_STORAGE_KEY = 'jump:appearance'
export const DEFAULT_THEME_ID = 'default'
export const SPACETIME_THEME_ID = 'spacetime'
export const VERCEL_THEME_ID = 'vercel'
export const THEME_CATALOG: readonly ThemeDefinition[] = [
  { id: DEFAULT_THEME_ID, label: 'Default', themeColor: '#0a0e13' },
  { id: SPACETIME_THEME_ID, label: 'Spacetime', themeColor: '#202126' },
  { id: VERCEL_THEME_ID, label: 'Vercel', themeColor: '#000000' },
]
export const DEFAULT_APPEARANCE: AppearancePreferences = { themeId: DEFAULT_THEME_ID }

const themeIds = new Set(THEME_CATALOG.map(theme => theme.id))
const themeIdPattern = /^[a-z0-9][a-z0-9_-]{0,39}$/

export function shouldShowThemeSwitcher(): boolean {
  return THEME_CATALOG.length > 1
}

export function isKnownThemeId(value: unknown): value is string {
  return typeof value === 'string' && themeIds.has(value)
}

export function normalizeThemeId(value: unknown): string {
  if (typeof value !== 'string') return DEFAULT_THEME_ID
  if (!themeIdPattern.test(value)) return DEFAULT_THEME_ID
  return themeIds.has(value) ? value : DEFAULT_THEME_ID
}

export function normalizeAppearance(value: unknown): AppearancePreferences {
  if (!value || typeof value !== 'object') return DEFAULT_APPEARANCE
  const record = value as Record<string, unknown>
  return { themeId: normalizeThemeId(record.theme_id ?? record.themeId) }
}

export function serializeAppearance(appearance: AppearancePreferences): { theme_id: string } {
  return { theme_id: normalizeThemeId(appearance.themeId) }
}

export function themeDefinition(themeId: string): ThemeDefinition {
  return THEME_CATALOG.find(theme => theme.id === normalizeThemeId(themeId)) ?? THEME_CATALOG[0]
}

function getLocalStorage(): StorageLike | null {
  try {
    if (typeof localStorage === 'undefined') return null
    return localStorage
  } catch {
    return null
  }
}

export function readCachedAppearance(storage: StorageLike | null = getLocalStorage()): AppearancePreferences {
  if (!storage) return DEFAULT_APPEARANCE
  try {
    const raw = storage.getItem(APPEARANCE_STORAGE_KEY)
    if (!raw) return DEFAULT_APPEARANCE
    return normalizeAppearance(JSON.parse(raw))
  } catch {
    return DEFAULT_APPEARANCE
  }
}

export function writeCachedAppearance(
  appearance: AppearancePreferences,
  storage: StorageLike | null = getLocalStorage(),
): void {
  if (!storage) return
  try {
    storage.setItem(APPEARANCE_STORAGE_KEY, JSON.stringify(serializeAppearance(appearance)))
  } catch {
    // Best-effort cache only. Server persistence remains the source of truth.
  }
}

export function applyAppearance(appearance: AppearancePreferences, doc: Document | null = typeof document !== 'undefined' ? document : null): void {
  if (!doc) return
  const theme = themeDefinition(appearance.themeId)
  doc.documentElement.dataset.theme = theme.id
  const meta = doc.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
  meta?.setAttribute('content', theme.themeColor)
}
