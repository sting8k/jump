import { render } from 'preact'
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { LocationProvider, Router, Route, lazy, useLocation } from 'preact-iso'
import '@xterm/xterm/css/xterm.css'
import './styles.css'

import { ReplayView } from './replay-view'
import { TerminalView } from './terminal'
import { useArrivalPulse } from './use-arrival-pulse'
import { Sidebar } from './sidebar'
import type { DotState } from './store'
import { usePresence } from './use-presence'
import type { NotifyMessage } from './presence'

import type { Session } from './types'
import { ManageProjectsModal } from './manage-projects'
import { SettingsModal } from './settings-modal'
import { ProjectHub } from './project-hub'
import { Home } from './home'
import { LaunchButton } from './launcher'
import { IconActivity, IconAlert, IconDots, IconHelp, IconMoon, IconRestart, IconSun } from './icons'
import { installCopySession } from './mock-data/export-session'
import { isCoarsePointerDevice } from './input-device'
import { fetchHostActions, requestDisplaySleep, type DisplaySleepCapability } from './host-actions'
import { releaseUpdateBadge } from './release-updates'
import { ThemeMenuOptions } from './theme-switcher'
import {
  TERMINAL_FONT_SIZE_MAX,
  TERMINAL_FONT_SIZE_MIN,
  TERMINAL_FONT_SIZE_STEP,
  adjustTerminalFontSize,
  loadTerminalFontSize,
  saveTerminalFontSize,
} from './terminal-font-size'

import {
  sessions, connState, selected, selectedId, view, health, peers,
  terminalOptions, keybinds, macCommandIsCtrl,
  backgroundActivity, unreadCount,
  urlPath,
  initStore, setNavigate, navigateToSession,
  dismissSession, resumeSession, restartSession,
  sessionStaleness,
} from './store'

// Lazy-loaded routes (code-split, not bundled with the main app)
const InputDiagnostics = lazy(() => import('./input-diagnostics'))

// ── Config ──

const USE_MOCK = import.meta.env.VITE_MOCK === '1' || location.search.includes('mock')

// Mock mode: hide close buttons and other interactive chrome via CSS.
if (USE_MOCK) document.documentElement.classList.add('mock-mode')

// Debug: __jumpCopySession() in devtools console
installCopySession()

// ── Components ──

function countPtySessions(items: readonly Session[]): { alive: number; dead: number } {
  let alive = 0
  let dead = 0
  for (const item of items) {
    if (item.alive) alive++
    else dead++
  }
  return { alive, dead }
}

type DisplaySleepRequestState = 'idle' | 'checking' | 'requesting' | 'sent' | 'failed'

function displaySleepStateLabel(capability: DisplaySleepCapability | null): string {
  if (!capability) return 'unknown'
  return capability.state
}

function displaySleepTag(capability: DisplaySleepCapability | null, requestState: DisplaySleepRequestState): string {
  if (requestState === 'checking') return 'checking'
  if (requestState === 'requesting') return 'sending'
  if (requestState === 'failed') return 'failed'
  if (!capability) return 'unknown'
  if (capability.available) {
    const state = displaySleepStateLabel(capability)
    return requestState === 'sent' ? `sent · ${state}` : state
  }
  if (capability.status === 'unsupported') return 'unsupported'
  return 'unavailable'
}

function displaySleepTagClass(capability: DisplaySleepCapability | null, requestState: DisplaySleepRequestState): string {
  if (requestState === 'failed') return 'error'
  if (requestState === 'checking' || requestState === 'requesting') return 'pending'
  if (!capability) return 'unknown'
  if (!capability.available) return ''
  if (capability.state === 'unknown') return 'unknown'
  return 'ok'
}

function DisplaySleepStatusIcon({ capability, requestState }: { capability: DisplaySleepCapability | null, requestState: DisplaySleepRequestState }) {
  if (requestState === 'checking' || requestState === 'requesting') return <IconActivity class="session-menu-action-tag-icon" />
  if (requestState === 'failed') return <IconAlert class="session-menu-action-tag-icon" />
  if (!capability) return <IconHelp class="session-menu-action-tag-icon" />
  if (!capability.available) return <IconAlert class="session-menu-action-tag-icon" />
  if (capability.state === 'awake') return <IconSun class="session-menu-action-tag-icon" />
  if (capability.state === 'asleep') return <IconMoon class="session-menu-action-tag-icon" />
  return <IconHelp class="session-menu-action-tag-icon" />
}

function displaySleepTitle(capability: DisplaySleepCapability | null): string {
  if (!capability) return 'Checking display sleep support…'
  if (capability.available) return `Sleep the host display (state: ${displaySleepStateLabel(capability)})`
  return capability.reason || 'Display sleep is not available on this host'
}

function MainHeader({ session, terminalFontSize, onTerminalFontSizeChange, onRestart }: {
  session: Session | null
  terminalFontSize: number
  onTerminalFontSizeChange: (delta: number) => void
  onRestart?: () => void
}) {
  if (!session) {
    return (
      <div class="main-header">
        <div class="main-header-title">
          jump
        </div>
        <div class="main-header-right">
          <SessionMenu
            session={null}
            terminalFontSize={terminalFontSize}
            onTerminalFontSizeChange={onTerminalFontSizeChange}
          />
        </div>
      </div>
    )
  }

  const shortCwd = session.cwd.replace(/^\/home\/[^/]+/, '~')
  const ptyCounts = countPtySessions(sessions.value)

  return (
    <div class="main-header">
      <div class="main-header-left">
        <div class="main-header-title">
          {session.title}
        </div>
        <div class="main-header-meta">
          <span class="main-header-cwd">{shortCwd}</span>
        </div>
      </div>
      <div class="main-header-right">
        {session.status && session.status.label && (
          <div class={`main-header-status ${session.status.error ? 'error' : session.status.working ? 'working' : ''}`}>
            <span
              class={`session-dot ${session.status.error ? 'error' : session.status.working ? 'working' : 'idle'}`}
              style={{ width: 5, height: 5 }}
            />
            {session.status.label}
          </div>
        )}
        <div
          class="main-header-pty-count"
          title={`PTY sessions: ${ptyCounts.alive} alive, ${ptyCounts.dead} dead`}
          aria-label={`PTY sessions: ${ptyCounts.alive} alive, ${ptyCounts.dead} dead`}
        >
          <IconActivity class="main-header-pty-icon" />
          <span class="main-header-pty-label">Active PTYs</span>
          <strong class="main-header-pty-live">{ptyCounts.alive}</strong>
          <span class="main-header-pty-dot" />
          <span class="main-header-pty-dead"><strong>{ptyCounts.dead}</strong><span class="main-header-pty-dead-label"> dead</span></span>
        </div>
        <SessionMenu
          session={session}
          onRestart={onRestart}
          terminalFontSize={terminalFontSize}
          onTerminalFontSizeChange={onTerminalFontSizeChange}
        />
      </div>
    </div>
  )
}

function SessionMenu({ session, terminalFontSize, onTerminalFontSizeChange, onRestart }: {
  session: Session | null
  terminalFontSize: number
  onTerminalFontSizeChange: (delta: number) => void
  onRestart?: () => void
}) {
  const [open, setOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const healthVal = health.value
  const [displaySleep, setDisplaySleep] = useState<DisplaySleepCapability | null>(null)
  const [displaySleepRequestState, setDisplaySleepRequestState] = useState<DisplaySleepRequestState>('idle')

  // For remote sessions, compare against the peer's version (not the local
  // daemon's). Peers don't expose runner_hash, so only version comparison
  // is possible for remote sessions.
  const peerVersion = session?.peer
    ? peers.value.find(p => p.name === session.peer)?.version
    : undefined
  const compareTarget = session
    ? (session.peer ? (peerVersion ? { version: peerVersion } : null) : healthVal)
    : null
  const staleKind = session ? sessionStaleness(session, compareTarget) : null
  const updateBadge = releaseUpdateBadge(healthVal?.update_available)
  const triggerHasBadge = !!staleKind || !!updateBadge

  // Close on outside click or Escape.
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    const onClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('keydown', onKey)
    document.addEventListener('mousedown', onClick)
    return () => {
      document.removeEventListener('keydown', onKey)
      document.removeEventListener('mousedown', onClick)
    }
  }, [open])

  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    setDisplaySleepRequestState('checking')
    fetchHostActions(controller.signal).then(actions => {
      if (controller.signal.aborted) return
      setDisplaySleep(actions?.display_sleep ?? null)
      setDisplaySleepRequestState('idle')
    }).catch(() => {
      if (controller.signal.aborted) return
      setDisplaySleep(null)
      setDisplaySleepRequestState('failed')
    })
    return () => controller.abort()
  }, [open])

  const handleDisplaySleep = async () => {
    if (!displaySleep?.available || displaySleepRequestState === 'requesting') return
    setDisplaySleepRequestState('requesting')
    const next = await requestDisplaySleep()
    if (next) {
      setDisplaySleep(next)
      setDisplaySleepRequestState('sent')
    } else {
      setDisplaySleepRequestState('failed')
    }
  }

  const versionDisplay = session
    ? session.runner_version
      ? `v${session.runner_version}`
      : session.binary_hash
        ? session.binary_hash.slice(0, 8)
        : 'unknown'
    : ''

  const hasActions = !!session?.alive && !!onRestart
  const displaySleepDisabled = !displaySleep?.available
    || displaySleepRequestState === 'checking'
    || displaySleepRequestState === 'requesting'
  const displaySleepTagText = displaySleepTag(displaySleep, displaySleepRequestState)
  const displaySleepTagClassName = displaySleepTagClass(displaySleep, displaySleepRequestState)

  return (
    <div class="session-menu" ref={menuRef}>
      <button
        class={`session-menu-trigger${triggerHasBadge ? ' stale' : ''}`}
        onClick={() => setOpen(!open)}
        title={updateBadge ? `${updateBadge.label} — App menu` : 'App menu'}
        aria-expanded={open}
      >
        <IconDots class="session-menu-icon" />
        {triggerHasBadge && <span class="session-menu-badge" />}
      </button>
      {open && (
        <div class="session-menu-dropdown">
          {hasActions && (
            <>
              <button
                class="session-menu-action"
                onClick={() => { setOpen(false); onRestart!() }}
              >
                <IconRestart class="session-menu-action-icon" />
                <span>Restart session</span>
                {staleKind && <span class="session-menu-action-tag">outdated</span>}
              </button>
              <div class="session-menu-divider" />
            </>
          )}
          {updateBadge && (
            <>
              <div class="session-menu-section-title">Jump</div>
              <a
                class="session-menu-action release-update"
                href={updateBadge.href}
                target="_blank"
                rel="noopener noreferrer"
                title={updateBadge.title}
                aria-label={updateBadge.title}
                onClick={() => setOpen(false)}
              >
                <span class="session-menu-action-label">
                  <IconAlert class="session-menu-action-icon" />
                  <span>Update available</span>
                </span>
                <span class="session-menu-action-tag">{updateBadge.tag}</span>
              </a>
              <div class="session-menu-divider" />
            </>
          )}
          <div class="session-menu-section-title">Appearance</div>
          <ThemeMenuOptions onSelect={() => setOpen(false)} />
          <div class="session-menu-divider" />
          <div class="session-menu-section-title">Host</div>
          <button
            type="button"
            class="session-menu-action"
            onClick={handleDisplaySleep}
            disabled={displaySleepDisabled}
            title={displaySleepTitle(displaySleep)}
          >
            <span class="session-menu-action-label">
              <IconMoon class="session-menu-action-icon" />
              <span>Sleep display</span>
            </span>
            <span
              class={`session-menu-action-tag status-icon ${displaySleepTagClassName}`}
              title={displaySleepTagText}
              aria-label={`Display sleep status: ${displaySleepTagText}`}
            >
              <DisplaySleepStatusIcon capability={displaySleep} requestState={displaySleepRequestState} />
            </span>
          </button>
          {session && (
            <>
              <div class="session-menu-divider" />
              <div class="session-menu-section-title">Terminal</div>
              <div class="session-menu-font-row">
                <span class="session-menu-label">Font size</span>
                <div class="session-menu-font-controls" aria-label="Terminal font size">
                  <button
                    type="button"
                    class="session-menu-font-btn"
                    onClick={() => onTerminalFontSizeChange(-TERMINAL_FONT_SIZE_STEP)}
                    disabled={terminalFontSize <= TERMINAL_FONT_SIZE_MIN}
                    aria-label="Decrease terminal font size"
                  >
                    −
                  </button>
                  <span class="session-menu-font-value">{terminalFontSize}px</span>
                  <button
                    type="button"
                    class="session-menu-font-btn"
                    onClick={() => onTerminalFontSizeChange(TERMINAL_FONT_SIZE_STEP)}
                    disabled={terminalFontSize >= TERMINAL_FONT_SIZE_MAX}
                    aria-label="Increase terminal font size"
                  >
                    +
                  </button>
                </div>
              </div>
              <div class="session-menu-divider" />
              <div class="session-menu-section-title">Session info</div>
              <div class="session-menu-row">
                <span class="session-menu-label">Adapter</span>
                <span class="session-menu-value">{session.kind}</span>
              </div>
              <div class="session-menu-row">
                <span class="session-menu-label">Version</span>
                <span class={`session-menu-value${staleKind ? ' stale' : ''}`}>
                  {versionDisplay}
                </span>
              </div>
              {session.peer && (
                <div class="session-menu-row">
                  <span class="session-menu-label">Host</span>
                  <span class="session-menu-value">{session.peer}</span>
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}

// ── Mobile nav icons ─────────────────────────────────────────────────────────

const S = { fill: 'none', stroke: 'currentColor', 'stroke-width': '1.4', 'stroke-linecap': 'round' as const, 'stroke-linejoin': 'round' as const }

const IconUp    = () => <svg viewBox="0 0 14 14" width="16" height="16" {...S}><path d="M7 10V4m0 0-3 3m3-3 3 3"/></svg>
const IconDown  = () => <svg viewBox="0 0 14 14" width="16" height="16" {...S}><path d="M7 4v6m0 0-3-3m3 3 3-3"/></svg>
const IconLeft  = () => <svg viewBox="0 0 14 14" width="16" height="16" {...S}><path d="M10 7H4m0 0 3-3M4 7l3 3"/></svg>
const IconRight = () => <svg viewBox="0 0 14 14" width="16" height="16" {...S}><path d="M4 7h6m0 0-3-3m3 3-3 3"/></svg>

const IconWordLeft  = () => <svg viewBox="0 0 18 14" width="20" height="16" {...S}><line x1="3.5" y1="3" x2="3.5" y2="11"/><path d="M13 7H6m0 0 3-3M6 7l3 3"/></svg>
const IconWordRight = () => <svg viewBox="0 0 18 14" width="20" height="16" {...S}><line x1="14.5" y1="3" x2="14.5" y2="11"/><path d="M5 7h7m0 0-3-3m3 3-3 3"/></svg>
const IconSend = () => <svg viewBox="0 0 14 14" width="16" height="16" fill="currentColor" stroke="none"><path d="M3 2.5l8 4.5-8 4.5V8.5L7.5 7 3 5.5z"/></svg>
const IconPaste = () => <svg viewBox="0 0 14 14" width="16" height="16" {...S}><rect x="3" y="3" width="8" height="9" rx="1"/><path d="M5.5 3V2.5a1.5 1.5 0 0 1 3 0V3"/><path d="M7 7v3m0 0-1.5-1.5M7 10l1.5-1.5"/></svg>
const IconKeyboard = () => <svg viewBox="0 0 18 14" width="18" height="14" {...S}><rect x="2" y="3" width="14" height="8" rx="1.5"/><path d="M5 6h.01M8 6h.01M11 6h.01M14 6h.01M5 8.5h8"/></svg>

function MobileTerminalBar({
  canSend,
  ctrlArmed,
  altArmed,
  onMenu,
  onSend,
  onPaste,
  onToggleCtrl,
  onToggleAlt,
  onToggleKeyboard,
  keyboardActive,
}: {
  canSend: boolean
  ctrlArmed: boolean
  altArmed: boolean
  onMenu: () => void
  onSend: (data: string) => void
  onPaste: () => void
  onToggleCtrl: () => void
  onToggleAlt: () => void
  onToggleKeyboard: () => void
  keyboardActive: boolean
}) {
  // Read signals directly; no props needed for these.
  const bgActivity: DotState = backgroundActivity.value
  const unread = unreadCount.value
  const arrival = useArrivalPulse(bgActivity, unread)

  const keepFocus = (ev: Event) => ev.preventDefault()
  const tap = (seq: string) => { onSend(seq) }

  const [holdWordMode, setHoldWordMode] = useState(false)
  const holdTimer1   = useRef<ReturnType<typeof setTimeout>  | null>(null)
  const holdTimer2   = useRef<ReturnType<typeof setTimeout>  | null>(null)
  const holdInterval = useRef<ReturnType<typeof setInterval> | null>(null)
  const holdGen      = useRef(0)
  const tabHoldSent = useRef(false)

  const clearHold = () => {
    holdGen.current++
    if (holdTimer1.current)   { clearTimeout(holdTimer1.current);   holdTimer1.current   = null }
    if (holdTimer2.current)   { clearTimeout(holdTimer2.current);   holdTimer2.current   = null }
    if (holdInterval.current) { clearInterval(holdInterval.current); holdInterval.current = null }
    setHoldWordMode(false)
  }

  useEffect(() => () => clearHold(), [])

  const startTabHold = () => {
    const gen = holdGen.current
    tabHoldSent.current = false
    holdTimer1.current = setTimeout(() => {
      if (holdGen.current !== gen) return
      tabHoldSent.current = true
      tap('\x1b[Z')
    }, 360)
  }

  const finishTabHold = () => {
    const sent = tabHoldSent.current
    clearHold()
    if (!sent) tap('\t')
  }

  const startArrowHold = (arrowSeq: string, wordSeq: string) => {
    const gen = holdGen.current
    holdTimer1.current = setTimeout(() => {
      if (holdGen.current !== gen) return
      holdInterval.current = setInterval(() => tap(arrowSeq), 50)
      holdTimer2.current = setTimeout(() => {
        if (holdGen.current !== gen) return
        clearInterval(holdInterval.current!)
        holdInterval.current = null
        setHoldWordMode(true)
        tap(wordSeq)
        holdInterval.current = setInterval(() => tap(wordSeq), 180)
      }, 700)
    }, 400)
  }

  const showCtrl = ctrlArmed || holdWordMode

  return (
    <div class="mobile-bottom-bar" aria-label="Mobile terminal controls">
      <button
        class={`mobile-bottom-action menu-btn${bgActivity !== 'none' ? ` bg-${bgActivity}` : ''}${arrival ? ` bg-${arrival}` : ''}`}
        onClick={onMenu}
        title="Open sessions"
      >
        ☰
      </button>
      <div class="mobile-bottom-sep" />
      <div class="mobile-terminal-actions" role="toolbar" aria-label="Terminal keys" onMouseDown={keepFocus}>
        {(ctrlArmed || altArmed)
          ? <button class="mobile-bottom-action" disabled={!canSend} onClick={() => tap('\x1b[A')} title="Up arrow"><IconUp /></button>
          : <button class="mobile-bottom-action" disabled={!canSend} onClick={() => tap('\x1b')} title="Escape">esc</button>
        }
        {(ctrlArmed || altArmed)
          ? <button class="mobile-bottom-action" disabled={!canSend} onClick={() => tap('\x1b[B')} title="Down arrow"><IconDown /></button>
          : <button
              class="mobile-bottom-action"
              disabled={!canSend}
              onPointerDown={e => { e.currentTarget.setPointerCapture(e.pointerId); e.preventDefault(); startTabHold() }}
              onPointerUp={finishTabHold}
              onPointerCancel={clearHold}
              onContextMenu={e => e.preventDefault()}
              title="Tab (hold for Shift+Tab)"
            >tab</button>
        }
        <button
          class={`mobile-bottom-action ${showCtrl ? 'armed' : ''}`}
          disabled={!canSend}
          onClick={() => { if (holdWordMode) { clearHold(); } else { onToggleCtrl(); } }}
          title={showCtrl ? 'Ctrl armed for next typed key' : 'Arm Ctrl for next typed key'}
          aria-pressed={showCtrl}
        >
          ctrl
        </button>
        <button
          class={`mobile-bottom-action ${altArmed ? 'armed' : ''}`}
          disabled={!canSend}
          onClick={() => { onToggleAlt() }}
          title={altArmed ? 'Alt armed for next typed key' : 'Arm Alt for next typed key'}
          aria-pressed={altArmed}
        >
          alt
        </button>
        {([
          { seq: '\x1b[D', wordSeq: '\x1b[1;5D', title: 'Left arrow',  wordTitle: 'Word left',  Icon: IconLeft,  WordIcon: IconWordLeft  },
          { seq: '\x1b[C', wordSeq: '\x1b[1;5C', title: 'Right arrow', wordTitle: 'Word right', Icon: IconRight, WordIcon: IconWordRight },
        ] as const).map(({ seq, wordSeq, title, wordTitle, Icon, WordIcon }) => (
          <button
            class="mobile-bottom-action"
            disabled={!canSend}
            onPointerDown={e => { e.currentTarget.setPointerCapture(e.pointerId); e.preventDefault(); const s = showCtrl ? wordSeq : seq; tap(s); startArrowHold(s, wordSeq) }}
            onPointerUp={clearHold}
            onPointerCancel={clearHold}
            onContextMenu={e => e.preventDefault()}
            title={showCtrl ? wordTitle : `${title} (hold to repeat)`}
          >
            {showCtrl ? <WordIcon /> : <Icon />}
          </button>
        ))}
        {ctrlArmed
          ? <button class="mobile-bottom-action" disabled={!canSend} onClick={onPaste} title="Paste from clipboard"><IconPaste /></button>
          : <button class="mobile-bottom-action send-btn" disabled={!canSend} onClick={() => tap('\r')} title="Send"><IconSend /></button>
        }
        <button
          class={`mobile-bottom-action keyboard-btn ${keyboardActive ? 'active' : ''}`}
          disabled={!canSend}
          onPointerDown={e => { e.preventDefault(); onToggleKeyboard() }}
          title={keyboardActive ? 'Hide keyboard' : 'Show keyboard'}
          aria-pressed={keyboardActive}
        ><IconKeyboard /></button>
      </div>
    </div>
  )
}

// ── App ──

function NotificationToasts({
  notifications,
  onActivate,
  onDismiss,
}: {
  notifications: NotifyMessage[]
  onActivate: (msg: NotifyMessage) => void
  onDismiss: (id: string) => void
}) {
  if (notifications.length === 0) return null

  return (
    <div class="notification-toasts" role="status" aria-live="polite">
      {notifications.map(n => (
        <div class="notification-toast" key={n.id}>
          <button class="notification-toast-main" onClick={() => onActivate(n)}>
            <span class="notification-toast-title">{n.title}</span>
            <span class="notification-toast-body">{n.body}</span>
          </button>
          <button
            class="notification-toast-dismiss"
            onClick={() => onDismiss(n.id)}
            aria-label="Dismiss notification"
          >
            ×
          </button>
        </div>
      ))}
    </div>
  )
}

function App() {
  // Visual viewport tracking for keyboard-aware layout.
  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return
    const update = () => {
      document.documentElement.style.setProperty('--app-height', `${vv.height}px`)
    }
    update()
    vv.addEventListener('resize', update)
    return () => vv.removeEventListener('resize', update)
  }, [])

  // Wire the store's navigate function to preact-iso's router.
  const loc = useLocation()
  useEffect(() => {
    setNavigate((url, replace) => loc.route(url, replace))
    // Test-only navigation hook: routes to a session by ID. Used by
    // e2e/helpers.ts to drive the app from a known session ID, since
    // the post-refactor home page no longer auto-selects.
    //
    // Returns true only when navigation was actually dispatched.
    // Returns false until both the session and its project have
    // loaded, so callers (and waitForURL) can rely on the URL having
    // changed once this returns true.
    ;(window as any).__jumpNavigateToSession = (sessionId: string): boolean => {
      return navigateToSession(sessionId, true)
    }
  }, [loc])

  // Sync preact-iso's URL to the store signal on every navigation.
  // useLayoutEffect ensures urlPath updates before paint, so the view
  // computed reacts before the browser renders a stale frame.
  useLayoutEffect(() => {
    urlPath.value = loc.path
  }, [loc.path])

  // Initialize the store (SSE, data fetching, effects).
  useEffect(() => initStore(), [])

  // ── Local UI state (not shared, belongs to App) ──
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [manageProjectsOpen, setManageProjectsOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [ctrlArmed, setCtrlArmed] = useState(false)
  const [altArmed, setAltArmed] = useState(false)
  const [keyboardActive, setKeyboardActive] = useState(false)

  const terminalInputRef = useRef<((data: string) => void) | null>(null)
  const terminalFocusRef = useRef<(() => void) | null>(null)
  const terminalKeyboardToggleRef = useRef<(() => void) | null>(null)
  const terminalKeyboardHideRef = useRef<(() => void) | null>(null)
  const terminalPasteRef = useRef<(() => void) | null>(null)

  // Read signals.
  const viewVal = view.value
  const selId = selectedId.value
  const selectedVal = selected.value
  const sessionsVal = sessions.value
  const connVal = connState.value
  const termOpts = terminalOptions.value
  const keybindsVal = keybinds.value
  const macCtrl = macCommandIsCtrl.value
  const [terminalFontSize, setTerminalFontSize] = useState(() => loadTerminalFontSize(termOpts?.fontSize ?? 13))
  const handleTerminalFontSizeChange = useCallback((delta: number) => {
    setTerminalFontSize(current => saveTerminalFontSize(adjustTerminalFontSize(current, delta)))
  }, [])

  const presenceState = usePresence()

  // ── Resume ──
  const [resumingId, setResumingId] = useState<string | null>(null)

  const handleCloseSession = useCallback((session: Session) => {
    dismissSession(session.id)
  }, [])

  const handleResume = useCallback((id: string) => {
    setResumingId(id)
    resumeSession(id).catch(err => {
      console.error('resume failed:', err)
      setResumingId(prev => prev === id ? null : prev)
    })
  }, [])

  // Clear modifier state when selection changes. Desktop also focuses terminal;
  // touch devices must not auto-open the soft keyboard when opening a session.
  useEffect(() => {
    if (!selId) return
    setResumingId(null)
    setCtrlArmed(false)
    setAltArmed(false)
    if (!isCoarsePointerDevice()) requestAnimationFrame(() => terminalFocusRef.current?.())
  }, [selId])

  // When a resumed session comes alive, navigate to it.
  useEffect(() => {
    if (resumingId) {
      const sess = sessionsVal.find(s => s.id === resumingId)
      if (sess?.alive && sess?.socket_path) {
        navigateToSession(resumingId, true)
        setResumingId(null)
      }
    }
  }, [sessionsVal, resumingId])

  // Resume timeout.
  useEffect(() => {
    if (!resumingId) return
    const t = setTimeout(() => setResumingId(null), 10_000)
    return () => clearTimeout(t)
  }, [resumingId])

  const canAttach = !!selectedVal?.alive && (!!selectedVal?.socket_path || !!selectedVal?.peer) && !USE_MOCK

  // Clear modifiers when terminal isn't attachable.
  useEffect(() => {
    if (!canAttach) { setCtrlArmed(false); setAltArmed(false) }
  }, [canAttach])

  // ── Terminal callbacks ──
  const handleTerminalInputReady = useCallback((send: ((data: string) => void) | null) => {
    terminalInputRef.current = send
  }, [])
  const handleTerminalFocusReady = useCallback((focus: (() => void) | null) => {
    terminalFocusRef.current = focus
    if (!isCoarsePointerDevice()) focus?.()
  }, [])
  const handleKeyboardToggleReady = useCallback((toggle: (() => void) | null) => {
    terminalKeyboardToggleRef.current = toggle
  }, [])
  const handleKeyboardHideReady = useCallback((hide: (() => void) | null) => {
    terminalKeyboardHideRef.current = hide
  }, [])
  const handleKeyboardActiveChange = useCallback((active: boolean) => {
    setKeyboardActive(active)
  }, [])
  const handleToggleKeyboard = useCallback(() => { terminalKeyboardToggleRef.current?.() }, [])
  const handleHideKeyboard = useCallback(() => { terminalKeyboardHideRef.current?.() }, [])
  const handleMobileInput = useCallback((data: string) => { terminalInputRef.current?.(data) }, [])
  const handleTerminalPasteReady = useCallback((paste: (() => void) | null) => {
    terminalPasteRef.current = paste
  }, [])
  // The trigger encapsulates clipboard read, binary detection, upload,
  // and PTY emission. Mobile and desktop now share one paste code path,
  // so binary clipboard items work from the toolbar button too.
  const handleMobilePaste = useCallback(() => {
    terminalPasteRef.current?.()
  }, [])
  const handleToggleCtrl = useCallback(() => {
    if (!canAttach) return
    setCtrlArmed(armed => !armed)
  }, [canAttach])
  const handleCtrlConsumed = useCallback(() => { setCtrlArmed(false) }, [])
  const handleToggleAlt = useCallback(() => {
    if (!canAttach) return
    setAltArmed(armed => !armed)
  }, [canAttach])
  const handleAltConsumed = useCallback(() => { setAltArmed(false) }, [])

  return (
    <div class="app-layout">
      <Sidebar
        resumingId={resumingId}
        onCloseSession={handleCloseSession}
        onManageProjects={() => { handleHideKeyboard(); setSidebarOpen(false); setManageProjectsOpen(true) }}
        onOpenSettings={() => { handleHideKeyboard(); setSidebarOpen(false); setSettingsOpen(true) }}
        open={sidebarOpen}
        onClose={() => { handleHideKeyboard(); setSidebarOpen(false) }}
        onInteract={handleHideKeyboard}
      />

      <ManageProjectsModal
        open={manageProjectsOpen}
        onClose={() => setManageProjectsOpen(false)}
      />

      <SettingsModal
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
        notifPermission={presenceState.notifPermission}
        requestNotifPermission={presenceState.requestNotifPermission}
      />

      <NotificationToasts
        notifications={presenceState.inAppNotifications}
        onActivate={presenceState.activateInAppNotification}
        onDismiss={presenceState.dismissInAppNotification}
      />

      <div class="main-panel">
        {viewVal !== null && viewVal.kind !== 'project' && viewVal.kind !== 'home' && (
          <MainHeader
            session={selectedVal}
            terminalFontSize={terminalFontSize}
            onTerminalFontSizeChange={handleTerminalFontSizeChange}
            onRestart={selectedVal ? () => { restartSession(selectedVal.id).catch(err => console.error('restart failed:', err)) } : undefined}
          />
        )}

        {connVal === 'connecting' ? (
          <div class="state-message">
            <div class="state-icon">⋯</div>
            <div class="state-title">Connecting</div>
            <div class="state-subtitle">Reaching jumpd...</div>
          </div>
        ) : connVal === 'error' ? (
          <div class="state-message">
            <div class="state-icon" style={{ color: 'var(--status-error)' }}>⚠</div>
            <div class="state-title">Connection failed</div>
            <div class="state-subtitle">Could not reach jumpd. Is it running?</div>
            <button class="btn btn-primary" style={{ marginTop: 12 }} onClick={() => location.reload()}>
              Retry
            </button>
          </div>
        ) : viewVal?.kind === 'project' ? (
          <ProjectHub
            projectSlug={viewVal.projectSlug}
            onCloseSession={handleCloseSession}
          />
        ) : selectedVal && (canAttach || USE_MOCK) && termOpts && keybindsVal ? (
          <TerminalView
            session={selectedVal}
            terminalOptions={termOpts}
            keybinds={keybindsVal}
            macCommandIsCtrl={macCtrl}
            ctrlArmed={ctrlArmed}
            onCtrlConsumed={handleCtrlConsumed}
            altArmed={altArmed}
            onAltConsumed={handleAltConsumed}
            onInputReady={handleTerminalInputReady}
            onPasteReady={handleTerminalPasteReady}
            onFocusReady={handleTerminalFocusReady}
            onKeyboardToggleReady={handleKeyboardToggleReady}
            onKeyboardHideReady={handleKeyboardHideReady}
            onKeyboardActiveChange={handleKeyboardActiveChange}
            terminalFontSize={terminalFontSize}
          />
        ) : selectedVal && !selectedVal.alive && termOpts && !USE_MOCK ? (
          <ReplayView
            session={selectedVal}
            terminalOptions={termOpts}
            onResume={handleResume}
            resuming={resumingId === selectedVal.id}
          />
        ) : selectedVal ? (
          <div class="state-message">
            <div class="state-subtitle">Connecting…</div>
          </div>
        ) : (
          <Home />
        )}

        <MobileTerminalBar
          canSend={canAttach}
          ctrlArmed={ctrlArmed}
          altArmed={altArmed}
          onMenu={() => { handleHideKeyboard(); setSidebarOpen(true) }}
          onSend={handleMobileInput}
          onPaste={handleMobilePaste}
          onToggleCtrl={handleToggleCtrl}
          onToggleAlt={handleToggleAlt}
          onToggleKeyboard={handleToggleKeyboard}
          keyboardActive={keyboardActive}
        />
      </div>
    </div>
  )
}

render(
  <LocationProvider>
    <Router>
      <Route path="/_/input-diagnostics" component={InputDiagnostics} />
      <Route default component={App} />
    </Router>
  </LocationProvider>,
  document.getElementById('app')!,
)
