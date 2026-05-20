import { describe, expect, it } from 'vitest'
import {
  APPEARANCE_STORAGE_KEY,
  ATELIER_THEME_ID,
  DEFAULT_APPEARANCE,
  DEFAULT_THEME_ID,
  VERCEL_THEME_ID,
  normalizeAppearance,
  normalizeThemeId,
  readCachedAppearance,
  serializeAppearance,
  shouldShowThemeSwitcher,
  writeCachedAppearance,
} from './appearance'

class MemoryStorage {
  private items = new Map<string, string>()

  getItem(key: string): string | null {
    return this.items.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.items.set(key, value)
  }
}

describe('appearance preferences', () => {
  it('defaults to the current built-in theme', () => {
    expect(DEFAULT_APPEARANCE).toEqual({ themeId: DEFAULT_THEME_ID })
    expect(shouldShowThemeSwitcher()).toBe(true)
  })

  it('normalizes snake_case and camelCase inputs', () => {
    expect(normalizeAppearance({ theme_id: 'spacetime' })).toEqual({ themeId: 'spacetime' })
    expect(normalizeAppearance({ themeId: VERCEL_THEME_ID })).toEqual({ themeId: VERCEL_THEME_ID })
    expect(normalizeAppearance({ theme_id: ATELIER_THEME_ID })).toEqual({ themeId: ATELIER_THEME_ID })
  })

  it('falls back for unknown or unsafe theme ids', () => {
    expect(normalizeThemeId('future')).toBe(DEFAULT_THEME_ID)
    expect(normalizeThemeId('../default')).toBe(DEFAULT_THEME_ID)
    expect(normalizeAppearance({ theme_id: '../default' })).toEqual(DEFAULT_APPEARANCE)
  })

  it('stores a compact server-compatible cache payload', () => {
    const storage = new MemoryStorage()
    writeCachedAppearance({ themeId: 'spacetime' }, storage)

    expect(storage.getItem(APPEARANCE_STORAGE_KEY)).toBe('{"theme_id":"spacetime"}')
    expect(readCachedAppearance(storage)).toEqual({ themeId: 'spacetime' })
  })

  it('ignores invalid cached JSON', () => {
    const storage = new MemoryStorage()
    storage.setItem(APPEARANCE_STORAGE_KEY, '{ nope')

    expect(readCachedAppearance(storage)).toEqual(DEFAULT_APPEARANCE)
  })

  it('serializes unknown values back to default', () => {
    expect(serializeAppearance({ themeId: 'future' })).toEqual({ theme_id: 'default' })
  })
})
