/**
 * Mobile keyboard input fixes for xterm.js.
 *
 * Problem: mobile keyboards (iOS autocorrect, dictation, predictive text)
 * replace words in xterm's hidden textarea rather than appending. xterm.js
 * doesn't distinguish replacements from appends, so each replacement
 * re-sends text that was already on screen, causing cascading duplication.
 *
 * The replacement signal differs by platform:
 *
 *   iOS Safari: a single insertText (or insertReplacementText) with a
 *   non-collapsed selection (selectionStart < selectionEnd).
 *
 *   Android Chrome: a deleteContentBackward with non-collapsed selection,
 *   immediately followed by an insertText with collapsed selection. Same
 *   logical operation, split into two DOM events.
 *
 * Fix: two-phase interception.
 *
 *   beforeinput (textarea, capture): detect the replacement signal (iOS:
 *   non-collapsed selection on insertText; Android: deleteContentBackward
 *   with non-collapsed selection, carried forward to the next insertText).
 *   Send backspaces to erase from the replacement start to the end of the
 *   textarea.
 *
 *   input (container, capture): fires before xterm's handler on the textarea
 *   because capture goes parent-first. We stopImmediatePropagation() to
 *   prevent xterm from also sending ev.data, then send the replacement text
 *   plus the preserved suffix ourselves.
 *
 * Android has an additional complication: keydown events with keyCode 229
 * trigger xterm's CompositionHelper._handleAnyTextareaChanges, which uses
 * String.replace(oldValue, '') to diff the textarea. This works for pure
 * appends but produces garbage when the keyboard modifies the middle of the
 * string (the old value isn't a substring of the new value, so replace()
 * returns the entire textarea). We neutralize this by resetting
 * textarea.value to its pre-autocorrect state after sending the correct
 * data, so the deferred diff sees no change.
 *
 * This approach never calls preventDefault(), so it works regardless of
 * whether the browser considers beforeinput cancelable for the given
 * inputType and element type (a known cross-browser inconsistency).
 *
 * Assumption: the terminal cursor sits right after the last character in the
 * textarea. This holds for the normal mobile typing flow where replacements
 * fire immediately after typing. Mobile on-screen keyboards don't have arrow
 * keys, and autocorrect/dictation don't fire after cursor movement.
 *
 * See also: /_/input-diagnostics for collecting real event traces.
 */
import type { Terminal } from '@xterm/xterm'

type SendFn = (data: string) => void

interface PendingReplacement {
  newText: string
  suffix: string
  nextValue: string
  /** When set, reset textarea.value after sending to neutralize xterm's
   *  _handleAnyTextareaChanges deferred diff (Android keyCode-229 path). */
  resetValue?: string
}

/** Tracks a deleteContentBackward with non-collapsed selection so the
 *  immediately following insertText can be recognized as a replacement. */
interface TrackedDeletion {
  preDeleteValue: string
  deleteStart: number
  deleteEnd: number
}

/**
 * Attach a handler that intercepts mobile keyboard word-replacement events
 * and translates them into terminal-compatible input sequences.
 *
 * Must be called after `term.open()` so `term.textarea` exists.
 * `container` should be the parent element of xterm's textarea (needed to
 * intercept input events in the capture phase before xterm sees them).
 * `send` should be the raw PTY send function (not sendInput, to avoid
 * ctrl/alt modifier interference; same convention as paste).
 *
 * Returns a cleanup function.
 */
export function attachMobileInputHandler(
  term: Terminal,
  container: HTMLElement,
  send: SendFn,
): () => void {
  const textarea = term.textarea
  if (!textarea) return () => {}

  // Autocorrect / word-replacement is a mobile-keyboard concern (iOS,
  // Android). On desktop, xterm.js manages the textarea selection
  // internally and may leave non-collapsed ranges that our handler would
  // misinterpret as autocorrect replacements, sending spurious backspaces.
  // Track the pointer type dynamically so tablet-mode switches are handled.
  const pointerQuery = window.matchMedia('(pointer: coarse)')
  let isTouchPrimary = pointerQuery.matches
  const onPointerChange = () => { isTouchPrimary = pointerQuery.matches }
  pointerQuery.addEventListener('change', onPointerChange)

  let pending: PendingReplacement | null = null
  let trackedDeletion: TrackedDeletion | null = null
  let composing = false
  let resetSyncTimer: ReturnType<typeof setTimeout> | null = null

  /** Queue a replacement for phase 2 and send the necessary backspaces now. */
  const queueReplacement = (
    value: string,
    selStart: number,
    selEnd: number,
    newText: string,
    resetValue?: string,
  ) => {
    const suffix = value.substring(selEnd)
    send('\x7f'.repeat(value.length - selStart))
    pending = {
      newText,
      suffix,
      nextValue: value.substring(0, selStart) + newText + suffix,
      resetValue,
    }
  }

  const syncTextareaForTerminalData = (data: string) => {
    if (!isTouchPrimary || composing || pending) return

    let value = textarea.value
    let start = textarea.selectionStart ?? value.length
    let end = textarea.selectionEnd ?? start
    let changed = false

    for (const ch of data) {
      if (ch !== '\x7f' && ch !== '\b') continue

      if (start < end) {
        value = value.substring(0, start) + value.substring(end)
      } else if (start > 0) {
        value = value.substring(0, start - 1) + value.substring(end)
        start--
      }
      end = start
      changed = true
    }

    if (!changed) return
    textarea.value = value
    textarea.selectionStart = textarea.selectionEnd = start
  }

  const dataDisposable = term.onData(syncTextareaForTerminalData)

  /** Extract inserted text from a beforeinput event. */
  const resolveText = (ev: InputEvent) =>
    ev.data ?? ev.dataTransfer?.getData('text/plain') ?? ''

  // Phase 1: detect replacement and send backspaces.
  const onBeforeInput = (ev: InputEvent) => {
    if (!isTouchPrimary) return

    // Snapshot and clear tracked deletion at the top; only the
    // deleteContentBackward branch may re-set it below.
    if (composing) return
    const deletion = trackedDeletion
    trackedDeletion = null

    // Android autocorrect: the keyboard splits word corrections into
    // deleteContentBackward (non-collapsed) + insertText (collapsed).
    // Track the deletion so we can combine it with the following insert.
    if (ev.inputType === 'deleteContentBackward') {
      const start = textarea.selectionStart ?? 0
      const end = textarea.selectionEnd ?? start
      // Non-collapsed: potential Android autocorrect start. Track it.
      // Collapsed: normal backspace. Leave trackedDeletion null (already cleared).
      if (start < end) {
        trackedDeletion = { preDeleteValue: textarea.value, deleteStart: start, deleteEnd: end }
      }
      return
    }

    if (ev.inputType !== 'insertText' && ev.inputType !== 'insertReplacementText') return

    const start = textarea.selectionStart ?? 0
    const end = textarea.selectionEnd ?? start

    // Android autocorrect phase 2: insertText immediately after a tracked
    // deletion completes the replacement pair.
    if (deletion && start === end) {
      const newText = resolveText(ev)
      if (newText) queueReplacement(
        deletion.preDeleteValue, deletion.deleteStart, deletion.deleteEnd,
        newText, deletion.preDeleteValue,
      )
      return
    }

    // Collapsed selection = normal append, let xterm handle it.
    if (start === end) return

    // iOS / single-event replacement: insertText or insertReplacementText
    // with non-collapsed selection.
    const newText = resolveText(ev)
    if (newText) queueReplacement(textarea.value, start, end, newText)
  }

  const onCompositionStart = () => {
    composing = true
    pending = null
    trackedDeletion = null
  }

  const onCompositionEnd = () => {
    composing = false
    pending = null
    trackedDeletion = null
  }

  // Phase 2: intercept the input event before xterm, send replacement + suffix.
  // Registered on the container (parent) so capture phase fires before
  // xterm's capture-phase handler on the textarea itself.
  const onInput = (ev: Event) => {
    if (composing) {
      pending = null
      return
    }
    if (!pending) return
    const { newText, suffix, nextValue, resetValue } = pending
    pending = null

    // Prevent xterm's _inputEvent from also sending ev.data.
    ev.stopImmediatePropagation()

    send(newText + suffix)

    // Android: reset textarea to the pre-autocorrect value. xterm's
    // CompositionHelper._handleAnyTextareaChanges (triggered by keydown 229)
    // captured this same value as oldValue and will diff against it in a
    // deferred setTimeout(0). By restoring it, the diff sees no change.
    if (resetValue !== undefined) {
      if (resetSyncTimer !== null) clearTimeout(resetSyncTimer)
      // First restore the value xterm captured before this Android keyCode-229
      // replacement so its deferred diff sees no change. Then, after that
      // timeout has had a chance to run, put the textarea back in sync with the
      // logical terminal line. Without this second sync, later backspace + Telex
      // corrections use stale selection offsets and erase too much text.
      textarea.value = resetValue
      textarea.selectionStart = textarea.selectionEnd = resetValue.length
      resetSyncTimer = setTimeout(() => {
        textarea.value = nextValue
        textarea.selectionStart = textarea.selectionEnd = nextValue.length
        resetSyncTimer = null
      }, 0)
    }
  }

  textarea.addEventListener('compositionstart', onCompositionStart, { capture: true })
  textarea.addEventListener('compositionend', onCompositionEnd, { capture: true })
  textarea.addEventListener('beforeinput', onBeforeInput, { capture: true })
  container.addEventListener('input', onInput, { capture: true })

  return () => {
    pointerQuery.removeEventListener('change', onPointerChange)
    dataDisposable.dispose()
    if (resetSyncTimer !== null) clearTimeout(resetSyncTimer)
    textarea.removeEventListener('compositionstart', onCompositionStart, { capture: true })
    textarea.removeEventListener('compositionend', onCompositionEnd, { capture: true })
    textarea.removeEventListener('beforeinput', onBeforeInput, { capture: true })
    container.removeEventListener('input', onInput, { capture: true })
  }
}
