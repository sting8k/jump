import type { ProjectItem, Session } from './types'
import { matchSession } from './projects'

export type ActivityState = 'active' | 'fading'
export type DotState = 'working' | 'error' | 'unread' | 'active' | 'fading' | 'none'

export interface SessionDotOptions {
  selected?: boolean
  resuming?: boolean
}

const DOT_PRIORITY: DotState[] = ['error', 'working', 'unread', 'active', 'fading']

export function baseSessionDotState(
  session: Session,
  activity: ReadonlyMap<string, ActivityState>,
): DotState {
  if (session.alive && session.status?.error) return 'error'
  if (session.alive && session.status?.working) return 'working'
  if (session.unread) return 'unread'

  const act = activity.get(session.id)
  if (act === 'active') return 'active'
  if (act === 'fading') return 'fading'
  return 'none'
}

export function sessionDotState(
  session: Session,
  activity: ReadonlyMap<string, ActivityState>,
  opts: SessionDotOptions = {},
): DotState {
  const state = opts.resuming ? 'working' : baseSessionDotState(session, activity)

  // Foreground session attention is consumed by the terminal itself. Keep
  // working visible because it is live execution state, not just a badge.
  if (opts.selected && state !== 'working') return 'none'
  return state
}

export function summarizeDotStates(states: Iterable<DotState>): DotState {
  const seen = new Set(states)
  for (const state of DOT_PRIORITY) {
    if (seen.has(state)) return state
  }
  return 'none'
}

export function backgroundDotState(
  allSessions: Session[],
  activity: ReadonlyMap<string, ActivityState>,
  selectedSessionId: string | null,
): DotState {
  return summarizeDotStates(
    allSessions
      .filter(s => s.alive && s.id !== selectedSessionId)
      .map(s => sessionDotState(s, activity)),
  )
}

export function unreadSessionCount(allSessions: Session[], selectedSessionId: string | null): number {
  return allSessions.filter(s => s.id !== selectedSessionId && s.alive && s.unread).length
}

export function sessionIdsInProject(
  allSessions: Session[],
  projects: ProjectItem[],
  projectSlug: string,
): string[] {
  return allSessions
    .filter(s => matchSession(s, projects)?.slug === projectSlug)
    .map(s => s.id)
}
