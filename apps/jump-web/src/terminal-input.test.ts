import { describe, expect, it } from 'vitest'
import {
  beginTerminalComposition,
  createTerminalCompositionInputState,
  filterTerminalInputData,
  finishTerminalComposition,
  normalizeTerminalInput,
} from './terminal-input'

describe('normalizeTerminalInput', () => {
  it('normalizes decomposed Vietnamese text to NFC', () => {
    expect(normalizeTerminalInput('tiếng Việt')).toBe('tiếng Việt')
  })

  it('leaves terminal control sequences intact', () => {
    expect(normalizeTerminalInput('\x1b[A\r\x7f')).toBe('\x1b[A\r\x7f')
  })
})

describe('terminal composition input gate', () => {
  it('drops mutable pre-edit data and returns the committed text once', () => {
    const state = createTerminalCompositionInputState()

    beginTerminalComposition(state)

    expect(filterTerminalInputData(state, 'd')).toBeNull()
    expect(filterTerminalInputData(state, 'đ')).toBeNull()
    expect(finishTerminalComposition(state, 'đô')).toBe('đô')

    // xterm may emit the committed text immediately after compositionend; the
    // app already sent compositionend.data, so this duplicate must not reach PTY.
    expect(filterTerminalInputData(state, 'đô')).toBeNull()
    expect(finishTerminalComposition(state, 'đô')).toBeNull()

    // If xterm batches the commit plus a following character, only the suffix
    // should pass through because the commit was already sent on compositionend.
    state.suppressNextData = 'đô'
    expect(filterTerminalInputData(state, 'đôi')).toBe('i')

    // Normal input after that should pass through.
    expect(filterTerminalInputData(state, 'i')).toBe('i')
  })

  it('lets xterm send the commit when compositionend has no data', () => {
    const state = createTerminalCompositionInputState()

    beginTerminalComposition(state)
    expect(filterTerminalInputData(state, 'a')).toBeNull()
    expect(finishTerminalComposition(state, '')).toBeNull()

    expect(filterTerminalInputData(state, 'á')).toBe('á')
  })
})
