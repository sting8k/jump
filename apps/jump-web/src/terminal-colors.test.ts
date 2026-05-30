import { describe, expect, it } from 'vitest'
import { buildTerminalOptions } from './settings-schema'
import { resolvedTerminalBackground, terminalBackgroundFromOsc } from './terminal-colors'

describe('terminal dynamic colors', () => {
  it('parses OSC 11 rgb background colors', () => {
    expect(terminalBackgroundFromOsc('rgb:11/22/33')).toBe('#112233')
    expect(terminalBackgroundFromOsc('rgb:1111/2222/3333')).toBe('#112233')
    expect(terminalBackgroundFromOsc('rgb:0/f/8')).toBe('#00ff88')
  })

  it('parses hex background colors', () => {
    expect(terminalBackgroundFromOsc('#ABCDEF')).toBe('#abcdef')
    expect(terminalBackgroundFromOsc('#ace')).toBe('#aaccee')
  })

  it('ignores OSC reports and invalid colors', () => {
    expect(terminalBackgroundFromOsc('?')).toBeNull()
    expect(terminalBackgroundFromOsc('')).toBeNull()
    expect(terminalBackgroundFromOsc('rgb:xx/22/33')).toBeNull()
  })

  it('uses the configured terminal theme background as the fallback', () => {
    const options = buildTerminalOptions(null, { background: '#123456' })
    expect(resolvedTerminalBackground(options)).toBe('#123456')
  })
})
