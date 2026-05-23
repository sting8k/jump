import { describe, expect, it } from 'vitest'
import {
  backgroundDotState,
  projectDotState,
  sessionDotState,
  sessionIdsInProject,
  unreadSessionCount,
} from './attention'
import type { ProjectItem, Session } from './types'

function makeSession(overrides: Partial<Session> & { id: string }): Session {
  return {
    created_at: '2026-01-01T00:00:00Z',
    command: ['/bin/sh'],
    cwd: '/repo/app',
    kind: 'shell',
    alive: true,
    pid: 1,
    exit_code: null,
    started_at: '2026-01-01T00:00:00Z',
    exited_at: null,
    title: 'shell',
    subtitle: '',
    status: null,
    unread: false,
    resumable: false,
    socket_path: '/tmp/s.sock',
    ...overrides,
  }
}

describe('attention model', () => {
  it('prioritizes persistent session state over transient activity', () => {
    const am = new Map([['sess-1', 'active' as const]])

    expect(sessionDotState(makeSession({ id: 'sess-1', unread: true }), am)).toBe('unread')
    expect(sessionDotState(makeSession({ id: 'sess-1', status: { label: '', working: true } }), am)).toBe('working')
    expect(sessionDotState(makeSession({ id: 'sess-1', status: { label: 'failed', working: false, error: true } }), am)).toBe('error')
  })

  it('suppresses all dots for the selected session', () => {
    const am = new Map<string, 'active' | 'fading'>()

    expect(sessionDotState(makeSession({ id: 'sess-1', unread: true }), am, { selected: true })).toBe('none')
    expect(sessionDotState(makeSession({ id: 'sess-1', status: { label: '', working: true } }), am, { selected: true })).toBe('none')
    expect(sessionDotState(makeSession({ id: 'sess-1' }), am, { selected: true, resuming: true })).toBe('none')
  })

  it('summarizes project attention excluding only the selected session', () => {
    const am = new Map<string, 'active' | 'fading'>()
    const sessions = [
      makeSession({ id: 'selected', status: { label: '', working: true } }),
      makeSession({ id: 'background', unread: true }),
    ]

    expect(projectDotState(sessions, am, 'selected')).toBe('unread')
    expect(projectDotState(sessions, am, 'background')).toBe('working')
  })


  it('keeps background unread visible when another session is selected', () => {
    const am = new Map<string, 'active' | 'fading'>()
    const sessions = [
      makeSession({ id: 'selected' }),
      makeSession({ id: 'background', unread: true }),
    ]

    expect(backgroundDotState(sessions, am, 'selected')).toBe('unread')
    expect(unreadSessionCount(sessions, 'selected')).toBe(1)
  })

  it('resolves project session ids for foreground project activity clearing', () => {
    const projects: ProjectItem[] = [
      { slug: 'tilth', match: [{ path: '/repo/tilth' }] },
      { slug: 'other', match: [{ path: '/repo/other' }] },
    ]
    const sessions = [
      makeSession({ id: 'tilth-1', cwd: '/repo/tilth' }),
      makeSession({ id: 'other-1', cwd: '/repo/other' }),
    ]

    expect(sessionIdsInProject(sessions, projects, 'tilth')).toEqual(['tilth-1'])
  })
})
