import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { sessions, sessionsLoaded, projects, upsertSession, removeSession, markSessionRead, handleActivity, isSessionActive, isSessionFading, activityMap, activityGeneration, clearProjectActivity, sessionStaleness, peers, peerAppearance, urlPath, selectedId, navigateToSession, setNavigate, launchSession, removeProject, health, startHealthRefresh, HEALTH_REFRESH_MS, HEALTH_REFRESH_SETTLE_MS, appearance, setThemeId, notificationPreferences, setNotificationPreferences, initStore } from './store'
import { APPEARANCE_STORAGE_KEY } from './appearance'
import { DEFAULT_NOTIFICATION_PREFERENCES } from './notifications'
import type { Session } from './types'
import type { ProjectItem } from './types'

function makeSession(overrides: Partial<Session> & { id: string }): Session {
  return {
    created_at: '2026-01-01T00:00:00Z',
    command: ['/bin/sh'],
    cwd: '/home/user',
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
    runner_version: undefined,
    ...overrides,
  }
}

class MemoryStorage {
  private items = new Map<string, string>()

  getItem(key: string): string | null {
    return this.items.get(key) ?? null
  }

  setItem(key: string, value: string): void {
    this.items.set(key, value)
  }
}

// Reset signal state between tests.
beforeEach(() => {
  sessions.value = []
  projects.value = []
  sessionsLoaded.value = false
  urlPath.value = '/'
  health.value = null
  appearance.value = { themeId: 'default' }
})

describe('upsertSession', () => {
  it('inserts a new session and returns true', () => {
    const isNew = upsertSession({
      id: 'sess-1', alive: true, cwd: '/home/user',
      command: ['/bin/sh'], kind: 'shell',
    } as any)
    expect(isNew).toBe(true)
    expect(sessions.value).toHaveLength(1)
    expect(sessions.value[0].id).toBe('sess-1')
  })

  it('updates an existing session and returns false', () => {
    sessions.value = [makeSession({ id: 'sess-1', title: 'old' })]
    const isNew = upsertSession({
      id: 'sess-1', alive: true, title: 'new',
      cwd: '/home/user', command: ['/bin/sh'], kind: 'shell',
    } as any)
    expect(isNew).toBe(false)
    expect(sessions.value).toHaveLength(1)
    expect(sessions.value[0].title).toBe('new')
  })

  it('preserves other sessions during update', () => {
    sessions.value = [
      makeSession({ id: 'sess-1', title: 'first' }),
      makeSession({ id: 'sess-2', title: 'second' }),
    ]
    upsertSession({
      id: 'sess-1', alive: false, title: 'updated',
      cwd: '/home/user', command: ['/bin/sh'], kind: 'shell',
    } as any)
    expect(sessions.value).toHaveLength(2)
    expect(sessions.value[0].title).toBe('updated')
    expect(sessions.value[1].title).toBe('second')
  })

  it('rewrites URL when selected session slug changes', () => {
    const testProjects: ProjectItem[] = [
      { slug: 'myproject', match: [{ path: '/dev/project' }] },
    ]
    projects.value = testProjects
    sessionsLoaded.value = true
    sessions.value = [
      makeSession({ id: 'sess-1', cwd: '/dev/project', kind: 'pi', slug: 'fix-auth' }),
    ]
    // Simulate the session being selected via URL.
    urlPath.value = '/myproject/pi/fix-auth'
    expect(selectedId.value).toBe('sess-1')

    // SSE upserts with a new slug (e.g., /new changed the active file).
    upsertSession({
      id: 'sess-1', alive: true, cwd: '/dev/project', kind: 'pi',
      slug: 'refactor-login', command: ['pi'], title: 'pi',
    } as any)

    // URL should be atomically rewritten; session stays selected.
    expect(urlPath.value).toBe('/myproject/pi/refactor-login')
    expect(selectedId.value).toBe('sess-1')
  })

  it('does not rewrite URL when a non-selected session slug changes', () => {
    const testProjects: ProjectItem[] = [
      { slug: 'myproject', match: [{ path: '/dev/project' }] },
    ]
    projects.value = testProjects
    sessionsLoaded.value = true
    sessions.value = [
      makeSession({ id: 'sess-1', cwd: '/dev/project', kind: 'pi', slug: 'fix-auth' }),
      makeSession({ id: 'sess-2', cwd: '/dev/project', kind: 'pi', slug: 'old-slug' }),
    ]
    urlPath.value = '/myproject/pi/fix-auth'
    expect(selectedId.value).toBe('sess-1')

    // sess-2's slug changes, but it's not the selected session.
    upsertSession({
      id: 'sess-2', alive: true, cwd: '/dev/project', kind: 'pi',
      slug: 'new-slug', command: ['pi'], title: 'pi',
    } as any)

    // URL should be unchanged.
    expect(urlPath.value).toBe('/myproject/pi/fix-auth')
    expect(selectedId.value).toBe('sess-1')
  })
})

describe('removeSession', () => {
  it('removes the session with the given id', () => {
    sessions.value = [
      makeSession({ id: 'sess-1' }),
      makeSession({ id: 'sess-2' }),
    ]
    removeSession('sess-1')
    expect(sessions.value.map(s => s.id)).toEqual(['sess-2'])
  })

  it('is a no-op for unknown ids', () => {
    sessions.value = [makeSession({ id: 'sess-1' })]
    removeSession('ghost')
    expect(sessions.value).toHaveLength(1)
  })
})

describe('project mutations', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true })) })
  afterEach(() => { vi.restoreAllMocks() })

  it('removes a project locally after PUT succeeds', async () => {
    projects.value = [
      { slug: 'jump', match: [{ path: '/dev/jump' }] },
      { slug: 'fxproj', match: [{ path: '/dev/fxproj' }] },
    ]

    await removeProject('jump')

    expect(fetch).toHaveBeenCalledWith('/v1/projects', expect.objectContaining({ method: 'PUT' }))
    expect(projects.value.map(p => p.slug)).toEqual(['fxproj'])
  })
})

describe('appearance preferences', () => {
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true })) })
  afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.restoreAllMocks() })

  it('applies theme locally and saves it to the server', async () => {
    await setThemeId('vercel')

    expect(appearance.value).toEqual({ themeId: 'vercel' })
    expect(fetch).toHaveBeenCalledWith('/v1/frontend-preferences', expect.objectContaining({
      method: 'PATCH',
      body: JSON.stringify({ appearance: { theme_id: 'vercel' } }),
    }))
  })

  it('keeps the cached theme when frontend config fetch fails', async () => {
    vi.useFakeTimers()
    const doc = new EventTarget() as Document
    Object.defineProperty(doc, 'documentElement', { value: { dataset: {}, style: {} } })
    Object.defineProperty(doc, 'querySelector', { value: () => null })
    Object.defineProperty(doc, 'visibilityState', { configurable: true, value: 'visible' })
    vi.stubGlobal('document', doc)
    vi.stubGlobal('window', new EventTarget())
    class FakeEventSource extends EventTarget {
      constructor(public url: string) { super() }
      close() {}
    }
    vi.stubGlobal('EventSource', FakeEventSource)

    const storage = new MemoryStorage()
    storage.setItem(APPEARANCE_STORAGE_KEY, '{"theme_id":"spacetime"}')
    vi.stubGlobal('localStorage', storage)

    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/v1/frontend-config') return { ok: false, json: async () => ({}) }
      if (url === '/v1/projects') return { ok: true, json: async () => ({ ok: true, data: { configured: [], discovered: [], unmatched_active_count: 0 } }) }
      if (url === '/v1/sessions') return { ok: true, json: async () => ({ data: [] }) }
      if (url === '/v1/health') return { ok: true, json: async () => ({ data: null }) }
      if (url === '/v1/session-metrics') return { ok: false, json: async () => ({}) }
      return { ok: true, json: async () => ({}) }
    }))

    const cleanup = initStore()
    await Promise.resolve()
    await Promise.resolve()
    cleanup()
    vi.useRealTimers()

    expect(appearance.value).toEqual({ themeId: 'spacetime' })
    expect(storage.getItem(APPEARANCE_STORAGE_KEY)).toBe('{"theme_id":"spacetime"}')
  })
})

describe('notification preferences', () => {
  beforeEach(() => {
    notificationPreferences.value = DEFAULT_NOTIFICATION_PREFERENCES
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }))
  })
  afterEach(() => { vi.restoreAllMocks() })

  it('defaults notification channels off', () => {
    expect(notificationPreferences.value).toEqual(DEFAULT_NOTIFICATION_PREFERENCES)
  })

  it('saves notification preferences to the server', async () => {
    await setNotificationPreferences({
      ...DEFAULT_NOTIFICATION_PREFERENCES,
      inApp: true,
      ntfy: { ...DEFAULT_NOTIFICATION_PREFERENCES.ntfy, topicId: 'jump-topic', enabled: true },
    })

    expect(notificationPreferences.value).toEqual({
      ...DEFAULT_NOTIFICATION_PREFERENCES,
      inApp: true,
      ntfy: { ...DEFAULT_NOTIFICATION_PREFERENCES.ntfy, topicId: 'jump-topic', enabled: true },
    })
    expect(fetch).toHaveBeenCalledWith('/v1/frontend-preferences', expect.objectContaining({
      method: 'PATCH',
      body: JSON.stringify({
        notifications: {
          in_app: true,
          os: false,
          ntfy: {
            enabled: true,
            server_url: 'https://ntfy.sh',
            topic_id: 'jump-topic',
            send_details: false,
          },
        },
      }),
    }))
  })
})


describe('health refresh', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      json: async () => ({ data: { version: '1.0.0', update_available: 'v1.1.0' } }),
    }))
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('refreshes health after the daemon update checker has time to settle, then on interval', async () => {
    const cleanup = startHealthRefresh()

    expect(fetch).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(HEALTH_REFRESH_SETTLE_MS)
    expect(fetch).toHaveBeenCalledWith('/v1/health')
    expect(health.value?.update_available).toBe('v1.1.0')

    vi.mocked(fetch).mockClear()
    await vi.advanceTimersByTimeAsync(HEALTH_REFRESH_MS)
    expect(fetch).toHaveBeenCalledWith('/v1/health')

    vi.mocked(fetch).mockClear()
    cleanup()
    await vi.advanceTimersByTimeAsync(HEALTH_REFRESH_MS)
    expect(fetch).not.toHaveBeenCalled()
  })
})

describe('markSessionRead', () => {
  // Prevent the actual fetch from firing.
  beforeEach(() => { vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true })) })
  afterEach(() => { vi.restoreAllMocks() })

  it('clears unread flag on the target session', () => {
    sessions.value = [makeSession({ id: 'sess-1', unread: true })]
    markSessionRead('sess-1')
    expect(sessions.value[0].unread).toBe(false)
  })

  it('clears error flag from status', () => {
    sessions.value = [makeSession({
      id: 'sess-1',
      status: { label: 'failed', working: false, error: true },
    })]
    markSessionRead('sess-1')
    expect(sessions.value[0].status?.error).toBe(false)
    expect(sessions.value[0].status?.label).toBe('failed')
  })

  it('does not touch other sessions', () => {
    sessions.value = [
      makeSession({ id: 'sess-1', unread: true }),
      makeSession({ id: 'sess-2', unread: true }),
    ]
    markSessionRead('sess-1')
    expect(sessions.value[0].unread).toBe(false)
    expect(sessions.value[1].unread).toBe(true)
  })

  it('posts to the server', () => {
    sessions.value = [makeSession({ id: 'sess-1', unread: true })]
    markSessionRead('sess-1')
    expect(fetch).toHaveBeenCalledWith('/v1/sessions/sess-1/read', { method: 'POST' })
  })
})

describe('launchSession', () => {
  afterEach(() => { vi.restoreAllMocks() })

  it('refreshes sessions after launch so the sidebar does not depend on SSE', async () => {
    projects.value = [{ slug: 'proj', match: [{ path: '/dev/proj' }] }]
    sessions.value = []
    const launched = makeSession({ id: 'sess-new', cwd: '/dev/proj', created_at: '2026-01-01T00:00:01Z' })

    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (url === '/v1/launch') {
        expect(init?.method).toBe('POST')
        return { ok: true, json: async () => ({}), text: async () => '' }
      }
      if (url === '/v1/projects') {
        return {
          ok: true,
          json: async () => ({
            ok: true,
            data: { configured: projects.value, discovered: [], unmatched_active_count: 0 },
          }),
        }
      }
      if (url === '/v1/sessions') {
        return { ok: true, json: async () => ({ data: [launched] }) }
      }
      throw new Error(`unexpected fetch ${url}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    await launchSession('shell', { cwd: '/dev/proj' })

    expect(fetchMock).toHaveBeenCalledWith('/v1/launch', expect.objectContaining({ method: 'POST' }))
    expect(fetchMock).toHaveBeenCalledWith('/v1/projects')
    expect(fetchMock).toHaveBeenCalledWith('/v1/sessions')
    expect(sessions.value.map(s => s.id)).toEqual(['sess-new'])
  })
})

describe('activity tracking', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    // Reset the activity map to a clean state.
    activityMap.value = new Map()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('marks a session as active immediately', () => {
    handleActivity('sess-1')
    expect(isSessionActive('sess-1')).toBe(true)
    expect(isSessionFading('sess-1')).toBe(false)
  })

  it('transitions to fading after the active window', () => {
    handleActivity('sess-1')
    vi.advanceTimersByTime(3000)
    expect(isSessionActive('sess-1')).toBe(false)
    expect(isSessionFading('sess-1')).toBe(true)
  })

  it('clears completely after fade-out', () => {
    handleActivity('sess-1')
    vi.advanceTimersByTime(3000 + 800)
    expect(isSessionActive('sess-1')).toBe(false)
    expect(isSessionFading('sess-1')).toBe(false)
  })

  it('ignores activity for a read known session', () => {
    sessions.value = [makeSession({ id: 'sess-1', unread: false })]
    handleActivity('sess-1')
    expect(isSessionActive('sess-1')).toBe(false)
    expect(isSessionFading('sess-1')).toBe(false)
  })

  it('uses activity only to repulse an unread idle session', () => {
    sessions.value = [makeSession({ id: 'sess-1', unread: true, status: null })]
    const before = activityGeneration.value.get('sess-1') ?? 0
    handleActivity('sess-1')
    expect(activityGeneration.value.get('sess-1')).toBe(before + 1)
  })

  it('ignores activity while a session is still working', () => {
    sessions.value = [makeSession({ id: 'sess-1', unread: true, status: { label: '', working: true } })]
    handleActivity('sess-1')
    expect(isSessionActive('sess-1')).toBe(false)
    expect(isSessionFading('sess-1')).toBe(false)
  })


  it('increments activity generation on each activity event', () => {
    const before = activityGeneration.value.get('sess-1') ?? 0
    handleActivity('sess-1')
    handleActivity('sess-1')
    expect(activityGeneration.value.get('sess-1')).toBe(before + 2)
  })

  it('clears activity for sessions in a viewed project only', () => {
    projects.value = [
      { slug: 'tilth', match: [{ path: '/repo/tilth' }] },
      { slug: 'other', match: [{ path: '/repo/other' }] },
    ]
    sessions.value = [
      makeSession({ id: 'sess-tilth', cwd: '/repo/tilth', unread: true }),
      makeSession({ id: 'sess-other', cwd: '/repo/other', unread: true }),
    ]
    handleActivity('sess-tilth')
    handleActivity('sess-other')

    clearProjectActivity('tilth')

    expect(isSessionActive('sess-tilth')).toBe(false)
    expect(isSessionActive('sess-other')).toBe(true)
  })


  it('cleans activity state when a session is removed', () => {
    sessions.value = [makeSession({ id: 'sess-1' })]
    handleActivity('sess-1')
    expect(activityGeneration.value.has('sess-1')).toBe(true)
    removeSession('sess-1')
    expect(activityMap.value.has('sess-1')).toBe(false)
    expect(activityGeneration.value.has('sess-1')).toBe(false)
  })

  it('resets the timer when activity fires again', () => {
    handleActivity('sess-1')
    vi.advanceTimersByTime(2000) // still active
    handleActivity('sess-1')     // reset
    vi.advanceTimersByTime(2000) // 2s since reset, still active
    expect(isSessionActive('sess-1')).toBe(true)
  })
})

describe('sessionStaleness', () => {
  const h = { version: '1.2.0', runner_hash: 'aabbccdd1122' }

  it('returns null when health is null (not yet loaded)', () => {
    expect(sessionStaleness({ runner_version: '1.1.0' }, null)).toBeNull()
  })

  it('returns null when runner_version is absent (pre-version runner)', () => {
    expect(sessionStaleness({}, h)).toBeNull()
    expect(sessionStaleness({ binary_hash: 'aabbccdd1122' }, h)).toBeNull()
  })

  it("returns 'version' when runner version differs from daemon version", () => {
    expect(sessionStaleness({ runner_version: '1.1.0' }, h)).toBe('version')
    expect(sessionStaleness({ runner_version: '0.9.0' }, h)).toBe('version')
  })

  it('returns null when runner and daemon versions match and no hash info', () => {
    expect(sessionStaleness({ runner_version: '1.2.0' }, { version: '1.2.0' })).toBeNull()
  })

  it('returns null when versions and hashes both match', () => {
    expect(sessionStaleness(
      { runner_version: '1.2.0', binary_hash: 'aabbccdd1122' }, h,
    )).toBeNull()
  })

  it("returns 'hash' when versions match but hashes differ (dev-mode drift)", () => {
    expect(sessionStaleness(
      { runner_version: '1.2.0', binary_hash: 'deadbeef9999' }, h,
    )).toBe('hash')
  })

  it("returns 'version' not 'hash' when both differ (version takes priority)", () => {
    expect(sessionStaleness(
      { runner_version: '1.1.0', binary_hash: 'deadbeef9999' }, h,
    )).toBe('version')
  })

  it("returns null for 'dev'/'dev' version match with no hash available", () => {
    // Common in dev: both report "dev", hash unknown on health side
    expect(sessionStaleness(
      { runner_version: 'dev', binary_hash: 'aabbcc' },
      { version: 'dev' },
    )).toBeNull()
  })

  it("returns 'hash' for 'dev'/'dev' version match with differing hashes", () => {
    expect(sessionStaleness(
      { runner_version: 'dev', binary_hash: 'deadbeef' },
      { version: 'dev', runner_hash: 'aabbccdd' },
    )).toBe('hash')
  })

  it('returns null when compared against peer with matching version (no hash)', () => {
    // Remote sessions are compared against their peer version, which has
    // no runner_hash. Hash drift should not trigger a false positive.
    expect(sessionStaleness(
      { runner_version: '1.2.0', binary_hash: 'deadbeef9999' },
      { version: '1.2.0' },
    )).toBeNull()
  })

  it("returns 'version' when compared against peer with different version", () => {
    expect(sessionStaleness(
      { runner_version: '1.1.0' },
      { version: '1.2.0' },
    )).toBe('version')
  })
})

describe('navigateToSession', () => {
  // The e2e helper (e2e/helpers.ts) polls a test hook that wraps
  // navigateToSession and treats its return value as "the URL has
  // changed". If the contract regresses (e.g. someone makes the
  // function return void again, or fires navigate() without a project
  // match), the e2e suite goes flaky in CI under SSE-vs-REST races
  // between sessions and projects. These tests pin that contract.
  let navigateMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    navigateMock = vi.fn()
    setNavigate(navigateMock)
  })
  afterEach(() => {
    setNavigate(() => {})
  })

  it('returns false and does not navigate when the session is unknown', () => {
    projects.value = [{ slug: 'p', match: [{ path: '/dev/p' }] }]
    expect(navigateToSession('ghost')).toBe(false)
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('returns false and does not navigate when projects have not loaded', () => {
    sessions.value = [makeSession({ id: 'sess-1', cwd: '/dev/p' })]
    // projects.value left empty: simulates the SSE-vs-REST race where
    // sessions arrive before projects.
    expect(navigateToSession('sess-1')).toBe(false)
    expect(navigateMock).not.toHaveBeenCalled()
  })

  it('returns true and dispatches the project-prefixed URL once both are loaded', () => {
    projects.value = [{ slug: 'myproject', match: [{ path: '/dev/p' }] }]
    sessions.value = [makeSession({ id: 'sess-1', cwd: '/dev/p', kind: 'shell' })]
    expect(navigateToSession('sess-1', true)).toBe(true)
    expect(navigateMock).toHaveBeenCalledTimes(1)
    const [url, replace] = navigateMock.mock.calls[0]
    expect(url).toMatch(/^\/myproject\/shell\//)
    expect(replace).toBe(true)
  })
})

describe('peerAppearance', () => {
  afterEach(() => { peers.value = [] })

  it('computes unique single-char prefixes when first chars differ', () => {
    peers.value = [
      { name: 'dev', url: '', status: 'connected', session_count: 0 },
      { name: 'staging', url: '', status: 'connected', session_count: 0 },
    ]
    const map = peerAppearance.value
    expect(map.get('dev')!.label).toBe('D')
    expect(map.get('staging')!.label).toBe('S')
  })

  it('extends prefix to disambiguate shared first characters', () => {
    peers.value = [
      { name: 'dev', url: '', status: 'connected', session_count: 0 },
      { name: 'desktop', url: '', status: 'connected', session_count: 0 },
    ]
    const map = peerAppearance.value
    // 'dev' vs 'desktop': 'de' is shared, need 3 chars
    expect(map.get('dev')!.label).toBe('DEV')
    expect(map.get('desktop')!.label).toBe('DES')
  })

  it('uses full name when one name is a prefix of another', () => {
    peers.value = [
      { name: 'dev', url: '', status: 'connected', session_count: 0 },
      { name: 'development', url: '', status: 'connected', session_count: 0 },
    ]
    const map = peerAppearance.value
    // 'dev' is fully consumed before it diverges from 'development'
    expect(map.get('dev')!.label).toBe('DEV')
    expect(map.get('development')!.label).toBe('DEVE')
  })

  it('assigns stable colors by name hash, independent of list order', () => {
    peers.value = [
      { name: 'alpha', url: '', status: 'connected', session_count: 0 },
      { name: 'beta', url: '', status: 'connected', session_count: 0 },
    ]
    const color1 = peerAppearance.value.get('alpha')!.color
    // Reverse order: alpha's color should not change
    peers.value = [
      { name: 'beta', url: '', status: 'connected', session_count: 0 },
      { name: 'alpha', url: '', status: 'connected', session_count: 0 },
    ]
    expect(peerAppearance.value.get('alpha')!.color).toBe(color1)
  })
})
