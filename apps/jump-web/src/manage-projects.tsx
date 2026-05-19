import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks'
import { projects, discovered, sessions, removeProject, addProject, updateProjects } from './store'
import type { ProjectItem, MatchRule } from './types'
import { IconFolder, IconPlus, IconTrash } from './icons'
import {
  buildWorkspaceSuggestions,
  cleanWorkspacePath,
  fsCompletionSuggestions,
  hasProjectPath,
  isWorkspacePath,
  shortenPath,
  workspaceName,
  type FSCompletion,
  type WorkspaceSuggestion,
} from './workspace-suggestions'

// ── Rule description ──

/** Human-readable parts of a single match rule. */
interface RuleDescription {
  prefix?: string   // e.g. "Remote"
  label: string     // monospace part: path or URL
  qualifier: string // dimmed suffix: "on any host"
}

function describeRule(rule: MatchRule): RuleDescription {
  const hosts = rule.hosts?.length
    ? rule.hosts.join(', ')
    : 'any host'

  if (rule.path) {
    const suffix = rule.exact ? ' only' : ''
    return {
      label: `${rule.path}${suffix}`,
      qualifier: `on ${hosts}`,
    }
  }

  if (rule.remote) {
    return {
      prefix: 'Remote',
      label: rule.remote,
      qualifier: `in any directory on ${hosts}`,
    }
  }

  return { label: '(empty rule)', qualifier: '' }
}

// ── Drag-to-reorder ──

/** State tracked during a drag operation. */
interface DragState {
  /** Index of the item being dragged. */
  from: number
  /** Current insertion target (visual feedback). */
  over: number
}

// ── ManageProjectsModal ──

export function ManageProjectsModal({
  open,
  onClose,
}: {
  open: boolean
  onClose: () => void
}) {
  const [filter, setFilter] = useState('')
  const [manualError, setManualError] = useState('')
  const [fsSuggestions, setFsSuggestions] = useState<WorkspaceSuggestion[]>([])
  const [drag, setDrag] = useState<DragState | null>(null)
  const backdropRef = useRef<HTMLDivElement>(null)

  // Close on Escape
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  // Reset filter when opening
  useEffect(() => {
    if (open) { setFilter(''); setManualError('') }
  }, [open])

  // Close on backdrop click
  const handleBackdropClick = useCallback((e: MouseEvent) => {
    if (e.target === backdropRef.current) onClose()
  }, [onClose])

  const configured = projects.value
  const discoveredVal = discovered.value
  const sessionVal = sessions.value

  const inputPath = cleanWorkspacePath(filter)
  const lowerFilter = filter.toLowerCase().trim()
  const filterIsPath = isWorkspacePath(inputPath)
  const duplicatePath = filterIsPath && hasProjectPath(configured, inputPath)

  useEffect(() => {
    if (!open || !filterIsPath || duplicatePath) {
      setFsSuggestions([])
      return
    }

    const controller = new AbortController()
    const timer = setTimeout(async () => {
      try {
        const resp = await fetch(`/v1/fs/complete?path=${encodeURIComponent(inputPath)}`, { signal: controller.signal })
        if (!resp.ok) {
          setFsSuggestions([])
          return
        }
        const json = await resp.json()
        const data: FSCompletion[] = json?.data ?? []
        setFsSuggestions(fsCompletionSuggestions(data))
      } catch {
        if (!controller.signal.aborted) setFsSuggestions([])
      }
    }, 180)

    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [open, filterIsPath, duplicatePath, inputPath])

  const suggestions = useMemo(() => {
    return buildWorkspaceSuggestions({
      fsSuggestions,
      sessionItems: sessionVal,
      configured,
      discoveredItems: discoveredVal,
      query: filter,
    })
  }, [fsSuggestions, sessionVal, configured, discoveredVal, filter])

  const topSuggestions = suggestions.slice(0, lowerFilter ? 12 : 8)

  // ── Reorder handlers ──

  const handleDragStart = useCallback((idx: number) => {
    setDrag({ from: idx, over: idx })
  }, [])

  const handleDragOver = useCallback((idx: number) => {
    setDrag(prev => prev ? { ...prev, over: idx } : null)
  }, [])

  const handleDragEnd = useCallback(() => {
    if (!drag || drag.from === drag.over) {
      setDrag(null)
      return
    }
    const items = [...configured]
    const [moved] = items.splice(drag.from, 1)
    items.splice(drag.over, 0, moved)
    updateProjects(items)
    setDrag(null)
  }, [drag, configured])

  // ── Remove handler ──

  const handleRemove = useCallback((slug: string) => {
    removeProject(slug)
  }, [])

  // ── Complete path from suggestions ──

  const handleUseSuggestion = useCallback((s: WorkspaceSuggestion) => {
    setManualError('')
    setFilter(s.path)
  }, [])

  // ── Manual add by path ──

  const handleManualAdd = useCallback(() => {
    const path = inputPath
    if (!path) return
    if (!isWorkspacePath(path)) {
      setManualError('Use an absolute path, e.g. ~/src/project or /Users/me/project')
      return
    }
    if (duplicatePath) {
      setManualError('Workspace already exists')
      return
    }
    setManualError('')
    addProject({ paths: [path] })
    setFilter('')
  }, [inputPath, duplicatePath])

  const handleFilterKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.key === 'Tab' && topSuggestions[0]) {
      e.preventDefault()
      handleUseSuggestion(topSuggestions[0])
      return
    }
    if (e.key === 'Enter' && filterIsPath && !duplicatePath) handleManualAdd()
  }, [filterIsPath, duplicatePath, handleManualAdd, handleUseSuggestion, topSuggestions])

  if (!open) return null

  // Compute the visual order during drag for CSS.
  const dragItems = drag ? reorder(configured, drag.from, drag.over) : configured

  return (
    <div class="modal-backdrop" ref={backdropRef} onClick={handleBackdropClick}>
      <div class="modal-panel manage-projects-modal">
        <div class="modal-header">
          <div class="modal-title">Manage projects</div>
          <div class="modal-header-actions">
            <a
              class="mp-docs-link"
              href="https://github.com/sting8k/jump/blob/dev/apps/website/src/content/docs/reference/projects-json.md#match-rules"
              target="_blank"
              rel="noopener"
              title="How match rules work"
            >?</a>
            <button class="modal-close" onClick={onClose}>&times;</button>
          </div>
        </div>

        <div class="modal-body">
          {/* ── Configured projects ── */}
          <section class="mp-section">
            <div class="mp-section-label">Your projects</div>
            {configured.length > 0 ? (
              <div class="mp-project-list">
                {dragItems.map((project, i) => (
                  <ProjectRow
                    key={project.slug}
                    project={project}
                    index={i}
                    dragging={drag !== null && project.slug === configured[drag.from]?.slug}
                    dropTarget={drag !== null && drag.over === i && drag.from !== i}
                    onDragStart={handleDragStart}
                    onDragOver={handleDragOver}
                    onDragEnd={handleDragEnd}
                    onRemove={handleRemove}
                  />
                ))}
              </div>
            ) : (
              <div class="mp-empty-hint">
                No projects yet. Add one from the list below, or type a path.
              </div>
            )}
          </section>

          {/* ── Smart add ── */}
          <section class="mp-section">
            <div class="mp-section-label">
              Smart add
              {topSuggestions.length > 0 && (
                <span class="mp-section-count">
                  {topSuggestions.length} suggestions
                </span>
              )}
            </div>

            <div class="mp-filter-row">
              <input
                class="mp-filter-input"
                type="text"
                placeholder="Paste ~/src/project or filter recent workspaces..."
                value={filter}
                onInput={(e) => { setFilter((e.target as HTMLInputElement).value); setManualError('') }}
                onKeyDown={handleFilterKeyDown}
              />
              {filterIsPath && (
                <button
                  class="mp-manual-btn"
                  disabled={duplicatePath}
                  onClick={handleManualAdd}
                  title={duplicatePath ? 'Workspace already exists' : `Add ${inputPath}`}
                >
                  <IconPlus class="btn-icon" />
                  <span>Add</span>
                </button>
              )}
            </div>
            {filterIsPath && (
              <div class={`mp-input-preview${duplicatePath ? ' duplicate' : ''}`}>
                {duplicatePath ? 'Already added' : `Will add ${workspaceName(inputPath)}`}
                <span>{shortenPath(inputPath)}</span>
              </div>
            )}
            {manualError && <div class="mp-manual-error">{manualError}</div>}

            <div class="mp-discovered-scroll">
              {topSuggestions.map(s => (
                <SuggestionRow key={s.key} suggestion={s} onUse={handleUseSuggestion} />
              ))}
              {topSuggestions.length === 0 && lowerFilter && !filterIsPath && (
                <div class="mp-empty-hint">
                  No matches. Paste an absolute path like <code>~/src/project</code> to add it.
                </div>
              )}
              {topSuggestions.length === 0 && !lowerFilter && (
                <div class="mp-empty-hint">
                  No recent workspace dirs yet. Start a session in a repo, or paste a path above.
                </div>
              )}
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}

// ── Sub-components ──

function ProjectRow({
  project,
  index,
  dragging,
  dropTarget,
  onDragStart,
  onDragOver,
  onDragEnd,
  onRemove,
}: {
  project: ProjectItem
  index: number
  dragging: boolean
  dropTarget: boolean
  onDragStart: (i: number) => void
  onDragOver: (i: number) => void
  onDragEnd: () => void
  onRemove: (slug: string) => void
}) {
  const rules = project.match

  return (
    <div
      class={`mp-project-row${dragging ? ' mp-dragging' : ''}${dropTarget ? ' mp-drop-target' : ''}`}
      draggable
      onDragStart={(e) => {
        e.dataTransfer!.effectAllowed = 'move'
        e.dataTransfer!.setData('text/plain', '')
        onDragStart(index)
      }}
      onDragOver={(e) => {
        e.preventDefault()
        e.dataTransfer!.dropEffect = 'move'
        onDragOver(index)
      }}
      onDrop={(e) => {
        e.preventDefault()
        onDragEnd()
      }}
      onDragEnd={onDragEnd}
    >
      <span class="mp-drag-handle" title="Drag to reorder">&#x283F;</span>
      <div class="mp-project-info">
        <span class="mp-project-name"><IconFolder class="mp-row-icon" />{project.slug}</span>
        <div class="mp-project-rules">
          {rules.map((rule, i) => {
            const { prefix, label, qualifier } = describeRule(rule)
            const title = [prefix, label, qualifier].filter(Boolean).join(' ')
            return (
              <span key={i} class="mp-rule" title={title}>
                {prefix && <span class="mp-rule-qualifier">{prefix} </span>}
                <span class="mp-rule-label">{label}</span>
                {qualifier && <span class="mp-rule-qualifier"> {qualifier}</span>}
              </span>
            )
          })}
        </div>
      </div>
      <button
        class="mp-remove-btn"
        onClick={() => onRemove(project.slug)}
        title="Remove project"
        aria-label={`Remove project ${project.slug}`}
      >
        <IconTrash class="btn-icon" />
      </button>
    </div>
  )
}

function SuggestionRow({
  suggestion,
  onUse,
}: {
  suggestion: WorkspaceSuggestion
  onUse: (s: WorkspaceSuggestion) => void
}) {
  const badge = suggestion.source === 'active' ? 'active' : suggestion.source === 'recent' ? 'recent' : suggestion.source === 'fs' ? 'dir' : 'found'

  return (
    <div class="mp-discovered-row" onClick={() => onUse(suggestion)}>
      <div class="mp-project-info">
        <span class="mp-project-name">
          <IconFolder class="mp-row-icon" />
          {suggestion.name}
          {suggestion.activeCount > 0 && (
            <span class="mp-active-badge">{suggestion.activeCount}</span>
          )}
          <span class={`mp-source-badge ${suggestion.source}`}>{badge}</span>
        </span>
        <span class="mp-project-detail" title={suggestion.detail}>{shortenPath(suggestion.detail)}</span>
      </div>
      <span class="mp-add-label"><IconPlus class="mp-add-icon" />Use</span>
    </div>
  )
}

// ── Helpers ──

/** Reorder an array by moving item at `from` to position `to`. */
function reorder<T>(arr: T[], from: number, to: number): T[] {
  const result = [...arr]
  const [moved] = result.splice(from, 1)
  result.splice(to, 0, moved)
  return result
}
