/**
 * Sidebar: project folders, session items, and the navigation shell.
 *
 * Reads shared state directly from the store (signals). Only action
 * callbacks and the mobile open/close toggle are passed as props.
 */

import { useState, useCallback } from 'preact/hooks'
import { sessionPath } from './routing'
import { LaunchButton } from './launcher'
import { useArrivalPulse } from './use-arrival-pulse'
import {
  folders, selectedId, currentProjectSlug,
  activityMap, activityGeneration, unmatchedActiveCount, projects, connState,
  updateProjects, reorderSessions,
  type DotState,
} from './store'
import { PeerLabel } from './peer-label'
import { IconBattery, IconCpu, IconFolder, IconMemory, IconSettings } from './icons'
import { formatBytes, formatPercent, useHostMetrics, type HostBattery } from './host-metrics'
import type { Session, Folder, ProjectItem } from './types'

// ── Types ──


// Re-export DotState so existing imports keep working.
export type { DotState }

// ── Helpers ──

/** Determine the dot indicator state for a session. */
function sessionDotState(session: Session, am: ReadonlyMap<string, 'active' | 'fading'>): DotState {
  if (session.alive && session.status?.error)   return 'error'
  if (session.alive && session.status?.working) return 'working'
  if (session.unread) return 'unread'
  const act = am.get(session.id)
  if (act === 'active') return 'active'
  if (act === 'fading') return 'fading'
  return 'none'
}

function formatSessionMemory(bytes?: number): string | null {
  if (typeof bytes !== 'number' || !Number.isFinite(bytes) || bytes <= 0) return null
  const mib = bytes / 1024 / 1024
  if (mib < 1024) return `${mib.toFixed(mib >= 100 ? 0 : 1)}M`
  const gib = mib / 1024
  return `${gib.toFixed(gib >= 10 ? 0 : 1)}G`
}

function formatBatteryState(state?: string): string {
  switch (state) {
    case 'charging': return 'chg'
    case 'discharging': return 'dis'
    case 'charged':
    case 'full': return 'full'
    case 'not charging': return 'hold'
    default: return state ? state.slice(0, 4) : ''
  }
}

function formatBatteryStatus(battery: HostBattery): string {
  const state = formatBatteryState(battery.state)
  return state ? `${formatPercent(battery.percent)} ${state}` : formatPercent(battery.percent)
}

function metricWidth(value: number): string {
  return formatPercent(Math.max(0, Math.min(value, 100)))
}


// ── Drag helpers ──

/** True on devices with a pointer (mouse/trackpad). Touch-only devices
 *  don't support the HTML5 drag API and setting draggable on them
 *  interferes with scroll. */
const canDrag = typeof matchMedia !== 'undefined' && matchMedia('(hover: hover)').matches

interface DragState {
  /** Index of the item being dragged (in the original array). */
  from: number
  /** Current visual insertion target. */
  over: number
}

// ── Components ──
function SidebarHostMetrics() {
  const metrics = useHostMetrics()

  if (!metrics) {
    return (
      <div class="sidebar-metrics tui" title="Host jumpd metrics loading">
        <div class="sidebar-metrics-row"><span class="sidebar-metrics-label"><IconCpu class="metric-icon" />cpu</span><strong>--.-%</strong></div>
        <div class="sidebar-metrics-row"><span class="sidebar-metrics-label"><IconMemory class="metric-icon" />ram</span><strong>--.-%</strong></div>
      </div>
    )
  }

  return (
    <div class="sidebar-metrics tui" title="Host jumpd CPU, RAM, and battery telemetry">
      <div class="sidebar-metrics-row">
        <span class="sidebar-metrics-label"><IconCpu class="metric-icon" />cpu</span>
        <strong>{formatPercent(metrics.cpu_percent)}</strong>
      </div>
      <div class="sidebar-metrics-bar"><i style={{ width: metricWidth(metrics.cpu_percent) }} /></div>
      <div class="sidebar-metrics-row">
        <span class="sidebar-metrics-label"><IconMemory class="metric-icon" />ram</span>
        <strong>{formatBytes(metrics.memory.used_bytes)} / {formatBytes(metrics.memory.total_bytes)}</strong>
      </div>
      <div class="sidebar-metrics-bar"><i style={{ width: metricWidth(metrics.memory.percent) }} /></div>
      {metrics.battery && <>
        <div class="sidebar-metrics-row">
          <span class="sidebar-metrics-label"><IconBattery class="metric-icon" />bat</span>
          <strong>{formatBatteryStatus(metrics.battery)}</strong>
        </div>
        <div class="sidebar-metrics-bar battery"><i style={{ width: metricWidth(metrics.battery.percent) }} /></div>
      </>}
    </div>
  )
}


function reorder<T>(arr: T[], from: number, to: number): T[] {
  const next = [...arr]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}

function SessionItem({
  session,
  href,
  selected,
  resuming,
  dotState: rawDotState,
  activityGeneration: activityPulseGeneration,
  dragging,
  dropTarget,
  onClose,
  onClick,
  onDragStart,
  onDragOver,
  onDragEnd,
}: {
  session: Session
  href: string
  selected: boolean
  resuming?: boolean
  dotState: DotState
  activityGeneration?: number
  dragging?: boolean
  dropTarget?: boolean
  onClose?: () => void
  /** Extra side-effects on click (e.g. close mobile sidebar). */
  onClick?: () => void
  onDragStart?: () => void
  onDragOver?: () => void
  onDragEnd?: () => void
}) {
  const effectiveDotState = resuming ? 'working' : rawDotState
  // Nothing is "unread" if you're already looking at it.
  const dotState = (selected && (effectiveDotState === 'error' || effectiveDotState === 'unread')) ? 'none' : effectiveDotState
  const arrival = useArrivalPulse(dotState, activityPulseGeneration)
  const sleeping = !session.alive && session.resumable
  const memory = formatSessionMemory(session.memory_rss_bytes)

  const cls = [
    'session-item',
    selected ? 'selected' : '',
    dragging ? 'session-dragging' : '',
    dropTarget ? 'session-drop-target' : '',
  ].filter(Boolean).join(' ')

  return (
    <a
      class={cls}
      href={href}
      draggable={canDrag && !!onDragStart}
      onClick={() => {
        onClick?.()
      }}
      onAuxClick={(e) => { if (e.button === 1 && onClose) { e.preventDefault(); onClose() } }}
      onDragStart={(e) => {
        e.dataTransfer!.effectAllowed = 'move'
        e.dataTransfer!.setData('text/plain', '')
        onDragStart?.()
      }}
      onDragOver={(e) => { e.preventDefault(); e.dataTransfer!.dropEffect = 'move'; onDragOver?.() }}
      onDrop={(e) => { e.preventDefault(); onDragEnd?.() }}
      onDragEnd={onDragEnd}
    >
      {sleeping
        ? <svg class="session-sleep-icon" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><title>Resumable</title><path d="M7 1h4l-4 4h4" /><path d="M1 5h5l-5 6h5" /></svg>
        : <span class={`session-dot-indicator ${dotState}${arrival ? ` ${arrival}` : ''}`} />
      }
      {session.peer && <PeerLabel name={session.peer} />}
      <div class="session-content">
        <div class="session-title-row">
          <span class="session-title">{session.title}</span>
        </div>
        {(session.status?.label || memory) && (
          <div class="session-meta">
            {session.status?.label && <span class="session-status-label">{session.status.label}</span>}
            {memory && <span class="session-memory" title="Runner + child process RSS">{memory}</span>}
          </div>
        )}
      </div>
      {onClose && (
        <button
          class="session-close-btn"
          onClick={(e) => { e.stopPropagation(); e.preventDefault(); onClose() }}
          title={session.alive ? 'Kill session' : 'Dismiss'}
        >
          ×
        </button>
      )}
    </a>
  )
}

function FolderGroup({
  folder,
  project,
  selId,
  curProjectSlug,
  resumingId,
  am,
  activityGen,
  onCloseSession,
  onClick,
}: {
  folder: Folder
  project: ProjectItem
  selId: string | null
  curProjectSlug: string | null
  resumingId: string | null
  am: ReadonlyMap<string, 'active' | 'fading'>
  activityGen: ReadonlyMap<string, number>
  onCloseSession: (session: Session) => void
  onClick?: () => void
}) {
  const [drag, setDrag] = useState<DragState | null>(null)

  const handleDragStart = useCallback((idx: number) => {
    setDrag({ from: idx, over: idx })
  }, [])

  const handleDragOver = useCallback((idx: number) => {
    setDrag(prev => prev ? { ...prev, over: idx } : null)
  }, [])

  const handleDragEnd = useCallback((visible: Session[]) => {
    if (!drag || drag.from === drag.over) {
      setDrag(null)
      return
    }
    const reordered = reorder(visible, drag.from, drag.over)
    const visibleKeys = reordered.map(s => s.slug || s.id)
    // Preserve keys of non-visible sessions (dead, non-resumable) at the end.
    const visibleSet = new Set(visibleKeys)
    const hidden = (project.sessions ?? []).filter(k => !visibleSet.has(k))
    reorderSessions(project.slug, [...visibleKeys, ...hidden])
    setDrag(null)
  }, [drag, project])

  const visible = folder.sessions.filter(s => s.alive || s.resumable)
  const displayItems = drag ? reorder(visible, drag.from, drag.over) : visible
  const isCurrent = curProjectSlug === folder.path
  return (
    <div class="folder">
      <div class="folder-header">
        <a
          class={`folder-name${isCurrent ? ' current' : ''}`}
          href={`/${folder.path}`}
          title={`Open ${folder.name} hub`}
          onClick={onClick}
        >
          <IconFolder class="folder-icon" />
          <span>{folder.name}</span>
        </a>
        <LaunchButton
          sessions={folder.sessions}
          selectedId={selId}
          fallbackCwd={folder.launchCwd ?? ''}
          className="folder-launch-btn"
        />
      </div>
      <div class="folder-sessions">
        {displayItems.map((s, i) => (
          <SessionItem
            key={s.id}
            session={s}
            href={sessionPath(folder.path, s)}
            selected={selId === s.id}
            resuming={resumingId === s.id}
            dotState={sessionDotState(s, am)}
            activityGeneration={activityGen.get(s.id) ?? 0}
            dragging={drag !== null && s.id === visible[drag.from]?.id}
            dropTarget={drag !== null && drag.over === i && drag.from !== i}
            onClose={() => onCloseSession(s)}
            onClick={onClick}
            onDragStart={() => handleDragStart(i)}
            onDragOver={() => handleDragOver(i)}
            onDragEnd={() => handleDragEnd(visible)}
          />
        ))}
      </div>
    </div>
  )
}

export function Sidebar({
  resumingId,
  onCloseSession,
  onManageProjects,
  onOpenSettings,
  open,
  onClose,
  onInteract,
}: {
  resumingId: string | null
  onCloseSession: (session: Session) => void
  onManageProjects: () => void
  onOpenSettings: () => void
  open: boolean
  onClose: () => void
  onInteract?: () => void
}) {
  // Read signals; component re-renders only when these values change.
  const foldersVal = folders.value
  const projectsVal = projects.value
  const selId = selectedId.value
  const curProjectSlug = currentProjectSlug.value
  const unmatchedCount = unmatchedActiveCount.value
  const am = activityMap.value
  const activityGen = activityGeneration.value
  const projectBySlug = new Map(projectsVal.map(p => [p.slug, p]))

  const totalVisible = foldersVal.reduce(
    (n, f) => n + f.sessions.filter(s => s.alive || s.resumable).length, 0,
  )
  const connected = connState.value === 'connected'
  const hasProjects = projectsVal.length > 0
  const isOnlyHomeProject = projectsVal.length === 1
    && projectsVal[0].slug === 'home'
    && projectsVal[0].match.some(r => r.path === '~' && r.exact)

  const seedHomeProject = async () => {
    if (projects.value.length === 0) {
      await updateProjects([{ slug: 'home', match: [{ path: '~', exact: true }] }])
    }
  }

  return (
    <>
      <div class={`sidebar-overlay ${open ? 'visible' : ''}`} onClick={onClose} />
      <aside class={`sidebar ${open ? 'open' : ''}`} onPointerDownCapture={onInteract}>
        <div class="sidebar-header">
          <a
            class="sidebar-logo"
            href="/"
            onClick={onClose}
          >jump</a>
          <div class="sidebar-header-actions">
            {connected && !hasProjects && (
              <LaunchButton
                className="sidebar-launch-btn"
                beforeLaunch={seedHomeProject}
                onLaunch={onClose}
              />
            )}
            <button
              class="sidebar-settings-btn sidebar-header-action-btn"
              onClick={onManageProjects}
              title="Manage projects"
              aria-label="Manage projects"
            >
              <IconFolder class="btn-icon" />
              {unmatchedCount > 0 && (
                <span class="sidebar-header-badge">{unmatchedCount}</span>
              )}
            </button>
          </div>
        </div>
        <div class="sidebar-scroll">
          {foldersVal.map(f => {
            const proj = projectBySlug.get(f.path)
            if (!proj) return null
            return (
              <FolderGroup
                key={f.path}
                folder={f}
                project={proj}
                selId={selId}
                curProjectSlug={curProjectSlug}
                resumingId={resumingId}
                am={am}
                activityGen={activityGen}
                onCloseSession={onCloseSession}
                onClick={onClose}
              />
            )
          })}
          {connected && totalVisible === 0 && !hasProjects && (
            <div class="sidebar-hint">
              Click <strong>+</strong> to start your first session.
            </div>
          )}
          {connected && isOnlyHomeProject && totalVisible > 0 && (
            <div class="sidebar-hint">
              <button class="sidebar-hint-link" onClick={onManageProjects}>
                Manage projects
              </button> to organize sessions by repo.
            </div>
          )}
        </div>
        <div class="sidebar-footer">
          <button class="manage-projects-btn sidebar-footer-action-btn" onClick={onOpenSettings}>
            <IconSettings class="btn-icon" />
            <span>Settings</span>
          </button>
          <SidebarHostMetrics />
        </div>
      </aside>
    </>
  )
}
