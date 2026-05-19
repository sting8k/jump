import { describe, expect, it } from 'vitest'
import type { DiscoveredProject, ProjectItem } from './types'
import { makeSession } from './test-helpers'
import {
  buildWorkspaceSuggestions,
  findWorkspaceSuggestionByPath,
  fsCompletionSuggestions,
  hasProjectPath,
  recentWorkspaceSuggestions,
} from './workspace-suggestions'

describe('workspace suggestions', () => {
  const configured: ProjectItem[] = [
    { slug: 'jump', match: [{ path: '~/Documents/Develope/pi-agent-ext/gmux' }] },
  ]

  it('matches non-path fuzzy queries against recent workspace roots', () => {
    const suggestions = buildWorkspaceSuggestions({
      sessionItems: [
        makeSession({
          id: 'pi-droid',
          cwd: '~/Documents/Develope/pi-agent-ext/pi-droid-styling/src',
          workspace_root: '~/Documents/Develope/pi-agent-ext/pi-droid-styling',
          created_at: '2026-05-19T10:00:00Z',
        }),
        makeSession({
          id: 'other',
          cwd: '~/Documents/Develope/landingpage',
          workspace_root: '~/Documents/Develope/landingpage',
          created_at: '2026-05-19T11:00:00Z',
        }),
      ],
      configured,
      discoveredItems: [],
      query: 'pi droid',
    })

    expect(suggestions.map(s => s.path)).toEqual([
      '~/Documents/Develope/pi-agent-ext/pi-droid-styling',
    ])
  })

  it('prefers filesystem completions for path-like queries', () => {
    const suggestions = buildWorkspaceSuggestions({
      fsSuggestions: fsCompletionSuggestions([
        { name: 'gmux', path: '~/Documents/Develope/pi-agent-ext/gmux' },
        { name: 'pi-droid-styling', path: '~/Documents/Develope/pi-agent-ext/pi-droid-styling' },
      ]),
      sessionItems: [
        makeSession({
          id: 'recent',
          cwd: '~/Documents/Develope/pi-agent-ext/pi-droid-styling',
          created_at: '2026-05-19T10:00:00Z',
        }),
      ],
      configured: [],
      discoveredItems: [],
      query: '~/Documents/Develope/pi-agent-ext/pi',
    })

    expect(suggestions[0]?.source).toBe('fs')
    expect(suggestions[0]?.path).toBe('~/Documents/Develope/pi-agent-ext/pi-droid-styling')
  })

  it('filters already configured project paths', () => {
    const suggestions = recentWorkspaceSuggestions([
      makeSession({
        id: 'configured',
        cwd: '~/Documents/Develope/pi-agent-ext/gmux/apps/jump-web',
        workspace_root: '~/Documents/Develope/pi-agent-ext/gmux',
      }),
    ], configured)

    expect(suggestions).toEqual([])
    expect(hasProjectPath(configured, '~/Documents/Develope/pi-agent-ext/gmux')).toBe(true)
  })

  it('carries remote metadata from discovered suggestions when ranking', () => {
    const discovered: DiscoveredProject[] = [{
      suggested_slug: 'pi-droid-styling',
      remote: 'github.com/sting8k/pi-droid-styling',
      paths: ['~/Documents/Develope/pi-agent-ext/pi-droid-styling'],
      session_count: 2,
      active_count: 1,
    }]

    const suggestions = buildWorkspaceSuggestions({
      sessionItems: [],
      configured: [],
      discoveredItems: discovered,
      query: 'droid',
    })

    expect(suggestions[0]).toMatchObject({
      path: '~/Documents/Develope/pi-agent-ext/pi-droid-styling',
      remote: 'github.com/sting8k/pi-droid-styling',
    })
  })

  it('finds an exact path suggestion after Tab completion so remote metadata survives add', () => {
    const discovered: DiscoveredProject[] = [{
      suggested_slug: 'remote-app',
      remote: 'github.com/sting8k/remote-app',
      paths: ['~/src/remote-app'],
      session_count: 1,
      active_count: 1,
    }]

    const suggestions = buildWorkspaceSuggestions({
      sessionItems: [],
      configured: [],
      discoveredItems: discovered,
      query: '~/src/remote-app',
    })

    expect(findWorkspaceSuggestionByPath(suggestions, '~/src/remote-app/')).toMatchObject({
      path: '~/src/remote-app',
      remote: 'github.com/sting8k/remote-app',
    })
  })
})
