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
  it.each([
    ['idle read', makeSession({ id: 'sess-1' }), new Map<string, 'active' | 'fading'>(), 'none'],
    ['active pulse', makeSession({ id: 'sess-1' }), new Map([['sess-1', 'active' as const]]), 'active'],
    ['fading pulse', makeSession({ id: 'sess-1' }), new Map([['sess-1', 'fading' as const]]), 'fading'],
    ['unread beats activity', makeSession({ id: 'sess-1', unread: true }), new Map([['sess-1', 'active' as const]]), 'unread'],
    ['working beats unread', makeSession({ id: 'sess-1', unread: true, status: { label: '', working: true } }), new Map<string, 'active' | 'fading'>(), 'working'],
    ['error beats working', makeSession({ id: 'sess-1', unread: true, status: { label: 'failed', working: true, error: true } }), new Map<string, 'active' | 'fading'>(), 'error'],
  ])('computes session dot state: %s', (_name, session, activity, want) => {
    expect(sessionDotState(session, activity)).toBe(want)
  })

  it.each([
    ['selected unread', makeSession({ id: 'sess-1', unread: true }), {}, 'none'],
    ['selected working', makeSession({ id: 'sess-1', status: { label: '', working: true } }), {}, 'none'],
    ['selected error', makeSession({ id: 'sess-1', status: { label: 'failed', working: false, error: true } }), {}, 'none'],
    ['selected resuming', makeSession({ id: 'sess-1' }), { resuming: true }, 'none'],
    ['background resuming', makeSession({ id: 'sess-1' }), { resuming: true }, 'working'],
  ])('applies foreground/resume rule: %s', (_name, session, opts, want) => {
    const selected = _name.startsWith('selected')
    expect(sessionDotState(session, new Map(), { selected, ...opts })).toBe(want)
  })

  it.each([
    ['error priority', [makeSession({ id: 'a', unread: true }), makeSession({ id: 'b', status: { label: 'failed', working: false, error: true } })], 'error'],
    ['working priority', [makeSession({ id: 'a', unread: true }), makeSession({ id: 'b', status: { label: '', working: true } })], 'working'],
    ['unread priority', [makeSession({ id: 'a', unread: true }), makeSession({ id: 'b' })], 'unread'],
    ['active priority', [makeSession({ id: 'a' }), makeSession({ id: 'b' })], 'active'],
  ])('summarizes background dot priority: %s', (_name, sessions, want) => {
    const activity = new Map<string, 'active' | 'fading'>([['a', 'active']])
    expect(backgroundDotState(sessions, activity, 'selected')).toBe(want)
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
