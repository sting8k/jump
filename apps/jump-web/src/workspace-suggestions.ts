import type { DiscoveredProject, ProjectItem, Session } from './types'

export interface FSCompletion {
  name: string
  path: string
}

export type WorkspaceSuggestionSource = 'active' | 'recent' | 'discovered' | 'fs'

export interface WorkspaceSuggestion {
  key: string
  name: string
  path: string
  remote?: string
  detail: string
  source: WorkspaceSuggestionSource
  activeCount: number
  sessionCount?: number
  lastSeenAt?: number
}

export function cleanWorkspacePath(input: string): string {
  const path = input.trim()
  if (path === '~' || path === '/') return path
  return path.replace(/\/+$/, '')
}

export function isWorkspacePath(path: string): boolean {
  return path === '~' || path.startsWith('~/') || path.startsWith('/')
}

export function workspaceName(path: string): string {
  const clean = cleanWorkspacePath(path)
  if (clean === '~') return 'home'
  const parts = clean.split('/').filter(Boolean)
  return parts[parts.length - 1] || clean.replace(/[^a-zA-Z0-9_-]+/g, '-') || 'workspace'
}

export function shortenPath(path: string): string {
  return path.replace(/^\/home\/[^/]+/, '~')
}

export function hasProjectPath(items: ProjectItem[], path: string): boolean {
  const clean = cleanWorkspacePath(path)
  return items.some(project => project.match.some(rule => rule.path && cleanWorkspacePath(rule.path) === clean))
}

export function fsCompletionSuggestions(items: FSCompletion[]): WorkspaceSuggestion[] {
  return items.map(item => ({
    key: `fs:${item.path}`,
    name: item.name,
    path: cleanWorkspacePath(item.path),
    detail: item.path,
    source: 'fs',
    activeCount: 0,
  }))
}

export function recentWorkspaceSuggestions(
  sessionItems: Session[],
  configured: ProjectItem[],
): WorkspaceSuggestion[] {
  const byPath = new Map<string, WorkspaceSuggestion>()

  for (const session of sessionItems) {
    const path = cleanWorkspacePath(session.workspace_root || session.cwd || '')
    if (!isWorkspacePath(path) || hasProjectPath(configured, path)) continue

    const seenAt = new Date(session.created_at || session.started_at || 0).getTime() || 0
    const existing = byPath.get(path)
    if (!existing) {
      byPath.set(path, {
        key: `recent:${path}`,
        name: workspaceName(path),
        path,
        detail: session.peer ? `${shortenPath(path)} on ${session.peer}` : shortenPath(path),
        source: session.alive ? 'active' : 'recent',
        activeCount: session.alive ? 1 : 0,
        sessionCount: 1,
        lastSeenAt: seenAt,
      })
      continue
    }

    existing.sessionCount = (existing.sessionCount ?? 0) + 1
    if (session.alive) {
      existing.activeCount += 1
      existing.source = 'active'
    }
    if (seenAt > (existing.lastSeenAt ?? 0)) {
      existing.lastSeenAt = seenAt
      existing.detail = session.peer ? `${shortenPath(path)} on ${session.peer}` : shortenPath(path)
    }
  }

  return [...byPath.values()].sort((a, b) => {
    if (a.activeCount !== b.activeCount) return b.activeCount - a.activeCount
    if ((a.lastSeenAt ?? 0) !== (b.lastSeenAt ?? 0)) return (b.lastSeenAt ?? 0) - (a.lastSeenAt ?? 0)
    if ((a.sessionCount ?? 0) !== (b.sessionCount ?? 0)) return (b.sessionCount ?? 0) - (a.sessionCount ?? 0)
    return a.name.localeCompare(b.name)
  })
}

export function discoveredSuggestions(
  items: DiscoveredProject[],
  configured: ProjectItem[],
): WorkspaceSuggestion[] {
  return items
    .map(d => {
      const path = cleanWorkspacePath(d.paths[0] || '')
      return {
        key: `discovered:${d.suggested_slug}`,
        name: d.suggested_slug,
        path,
        remote: d.remote,
        detail: d.remote || shortenPath(path),
        source: d.active_count > 0 ? 'active' as const : 'discovered' as const,
        activeCount: d.active_count,
        sessionCount: d.session_count,
      }
    })
    .filter(s => s.path && !hasProjectPath(configured, s.path))
}

interface WorkspaceSuggestionInput {
  fsSuggestions?: WorkspaceSuggestion[]
  sessionItems: Session[]
  configured: ProjectItem[]
  discoveredItems: DiscoveredProject[]
  query: string
}

export function buildWorkspaceSuggestions({
  fsSuggestions = [],
  sessionItems,
  configured,
  discoveredItems,
  query,
}: WorkspaceSuggestionInput): WorkspaceSuggestion[] {
  const cleanQuery = cleanWorkspacePath(query)
  const lowerQuery = query.toLowerCase().trim()
  const queryIsPath = isWorkspacePath(cleanQuery)
  const all = [
    ...fsSuggestions,
    ...recentWorkspaceSuggestions(sessionItems, configured),
    ...discoveredSuggestions(discoveredItems, configured),
  ]
  const seen = new Set<string>()
  const unique = all.filter(s => {
    const path = cleanWorkspacePath(s.path)
    if (!path || seen.has(path)) return false
    seen.add(path)
    s.path = path
    return true
  })

  if (!lowerQuery) return sortDefault(unique)

  const scored = unique
    .map(s => ({ suggestion: s, score: suggestionScore(s, lowerQuery, queryIsPath) }))
    .filter(item => item.score > 0)
    .sort((a, b) => {
      if (a.score !== b.score) return b.score - a.score
      if (a.suggestion.activeCount !== b.suggestion.activeCount) return b.suggestion.activeCount - a.suggestion.activeCount
      if ((a.suggestion.lastSeenAt ?? 0) !== (b.suggestion.lastSeenAt ?? 0)) {
        return (b.suggestion.lastSeenAt ?? 0) - (a.suggestion.lastSeenAt ?? 0)
      }
      return a.suggestion.name.localeCompare(b.suggestion.name)
    })

  return scored.map(item => item.suggestion)
}

function sortDefault(items: WorkspaceSuggestion[]): WorkspaceSuggestion[] {
  return [...items].sort((a, b) => {
    const rank = (s: WorkspaceSuggestion) => {
      if (s.source === 'fs') return 0
      if (s.source === 'active') return 1
      if (s.source === 'recent') return 2
      return 3
    }
    return rank(a) - rank(b)
      || b.activeCount - a.activeCount
      || (b.sessionCount ?? 0) - (a.sessionCount ?? 0)
      || (b.lastSeenAt ?? 0) - (a.lastSeenAt ?? 0)
      || a.name.localeCompare(b.name)
  })
}

function suggestionScore(
  suggestion: WorkspaceSuggestion,
  lowerQuery: string,
  queryIsPath: boolean,
): number {
  const haystack = normalizeSearchText([
    suggestion.name,
    suggestion.path,
    suggestion.detail,
    suggestion.remote ?? '',
  ].join(' '))
  const query = normalizeSearchText(lowerQuery)
  const tokens = query.split(/\s+/).filter(Boolean)
  if (tokens.length === 0) return 0

  let score = 0
  for (const token of tokens) {
    const index = haystack.indexOf(token)
    if (index < 0) return 0
    score += Math.max(25, 120 - index)
  }

  const name = normalizeSearchText(suggestion.name)
  const pathBase = normalizeSearchText(workspaceName(suggestion.path))
  if (name === query || pathBase === query) score += 260
  if (name.startsWith(query) || pathBase.startsWith(query)) score += 140
  if (haystack.includes(query)) score += 90

  if (queryIsPath && suggestion.source === 'fs') score += 220
  if (suggestion.source === 'active') score += 120
  else if (suggestion.source === 'recent') score += 70
  else if (suggestion.source === 'discovered') score += 40

  score += Math.min(suggestion.activeCount, 10) * 15
  score += Math.min(suggestion.sessionCount ?? 0, 10) * 6
  return score
}

function normalizeSearchText(input: string): string {
  return input.toLowerCase().replace(/[^a-z0-9/~._-]+/g, ' ').replace(/\s+/g, ' ').trim()
}
