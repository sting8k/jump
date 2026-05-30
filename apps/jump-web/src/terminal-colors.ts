import { DEFAULT_THEME_COLORS } from './config'
import type { ResolvedTerminalOptions } from './settings-schema'

function normalizeHexColor(value: string): string | null {
  const text = value.trim()
  if (/^#[0-9a-f]{6}$/i.test(text)) return text.toLowerCase()
  if (/^#[0-9a-f]{3}$/i.test(text)) {
    const [, r, g, b] = text.toLowerCase()
    return `#${r}${r}${g}${g}${b}${b}`
  }
  return null
}

function oscHexComponentToByte(component: string): string | null {
  if (!/^[0-9a-f]{1,4}$/i.test(component)) return null
  const value = Number.parseInt(component, 16)
  const max = Math.pow(16, component.length) - 1
  const byte = Math.round((value / max) * 255)
  return byte.toString(16).padStart(2, '0')
}

export function terminalBackgroundFromOsc(data: string): string | null {
  const text = data.trim()
  if (!text || text === '?') return null

  const hex = normalizeHexColor(text)
  if (hex) return hex

  const match = /^rgb:([0-9a-f]{1,4})\/([0-9a-f]{1,4})\/([0-9a-f]{1,4})$/i.exec(text)
  if (!match) return null

  const r = oscHexComponentToByte(match[1])
  const g = oscHexComponentToByte(match[2])
  const b = oscHexComponentToByte(match[3])
  if (!r || !g || !b) return null
  return `#${r}${g}${b}`
}

export function resolvedTerminalBackground(options: ResolvedTerminalOptions): string {
  const configured = options.theme.background
  return typeof configured === 'string' && configured.trim()
    ? configured.trim()
    : DEFAULT_THEME_COLORS.background ?? '#000000'
}
