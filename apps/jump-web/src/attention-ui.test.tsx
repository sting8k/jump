import { beforeEach, describe, expect, it } from 'vitest'
import render from 'preact-render-to-string'
import { Home } from './home'
import { ProjectHub } from './project-hub'
import { Sidebar } from './sidebar'
import { sessions, sessionsLoaded, projects, urlPath, peers } from './store'
import type { Session } from './types'

function makeSession(overrides: Partial<Session> & { id: string }): Session {
  return {
    created_at: '2026-01-01T00:00:00Z',
    command: ['/bin/sh'],
    cwd: '/repo/gmux',
    kind: 'pi',
    alive: true,
    pid: 1,
    exit_code: null,
    started_at: '2026-01-01T00:00:00Z',
    exited_at: null,
    title: 'gmux agent',
    subtitle: '',
    status: null,
    unread: false,
    resumable: false,
    socket_path: '/tmp/s.sock',
    ...overrides,
  }
}

beforeEach(() => {
  sessions.value = []
  sessionsLoaded.value = true
  projects.value = [{ slug: 'gmux', match: [{ path: '/repo/gmux' }] }]
  peers.value = []
  urlPath.value = '/'
})

describe('attention UI surfaces', () => {
  it('renders a workspace attention dot before the sidebar folder icon', () => {
    sessions.value = [makeSession({ id: 'sess-bg', unread: true })]

    const html = render(
      <Sidebar
        resumingId={null}
        onCloseSession={() => {}}
        onManageProjects={() => {}}
        onOpenSettings={() => {}}
        open={false}
        onClose={() => {}}
      />,
    )

    const dot = html.indexOf('folder-attention-dot session-dot-indicator unread')
    const icon = html.indexOf('folder-icon')
    expect(dot).toBeGreaterThanOrEqual(0)
    expect(icon).toBeGreaterThanOrEqual(0)
    expect(dot).toBeLessThan(icon)
  })

  it('renders a workspace attention dot on the home project card', () => {
    sessions.value = [makeSession({ id: 'sess-bg', status: { label: '', working: true } })]

    const html = render(<Home />)

    const dot = html.indexOf('home-project-attention-dot session-dot-indicator working')
    const icon = html.indexOf('home-card-icon')
    expect(dot).toBeGreaterThanOrEqual(0)
    expect(icon).toBeGreaterThanOrEqual(0)
    expect(dot).toBeLessThan(icon)
  })

  it('renders session attention dots inside the project hub', () => {
    sessions.value = [makeSession({ id: 'sess-bg', unread: true })]
    urlPath.value = '/gmux'

    const html = render(<ProjectHub projectSlug="gmux" onCloseSession={() => {}} />)

    expect(html).toContain('session-card-dot unread')
  })
})
