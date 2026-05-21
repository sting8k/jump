/** Helpers for text bytes emitted from xterm's input surface. */

/**
 * Normalize user text to NFC before it crosses the PTY boundary.
 *
 * Some Vietnamese IMEs emit decomposed sequences such as `a` + combining
 * acute. Shell line editors and terminal renderers handle the precomposed NFC
 * form more consistently, while terminal control sequences are ASCII and are
 * unaffected by Unicode normalization.
 */
export function normalizeTerminalInput(data: string): string {
  return data.normalize('NFC')
}

/**
 * Gates xterm's `onData` while an IME owns the hidden textarea.
 *
 * Mobile Vietnamese Telex can emit mutable pre-edit text (`d` → `đ` → `đô`)
 * before the IME commits. Sending those intermediate bytes to the PTY lets the
 * shell echo stale text and makes later composition updates look like missing
 * or overwritten characters. During composition, drop xterm's pre-edit `onData`
 * and send only the normalized committed text from `compositionend` or the final
 * non-composing `input` event. If xterm also emits that same committed text,
 * suppress the duplicate once.
 */
export interface TerminalCompositionInputState {
  composing: boolean
  suppressNextData: string | null
}

export function createTerminalCompositionInputState(): TerminalCompositionInputState {
  return { composing: false, suppressNextData: null }
}

export function beginTerminalComposition(state: TerminalCompositionInputState): void {
  state.composing = true
  state.suppressNextData = null
}

export function finishTerminalComposition(state: TerminalCompositionInputState, data: string): string | null {
  const normalized = normalizeTerminalInput(data)
  if (!state.composing) return null
  state.composing = false
  if (!normalized) return null
  state.suppressNextData = normalized
  return normalized
}

export function filterTerminalInputData(state: TerminalCompositionInputState, data: string): string | null {
  const normalized = normalizeTerminalInput(data)
  if (state.composing) return null
  if (state.suppressNextData && normalized.startsWith(state.suppressNextData)) {
    const rest = normalized.slice(state.suppressNextData.length)
    state.suppressNextData = null
    return rest || null
  }
  return normalized
}
