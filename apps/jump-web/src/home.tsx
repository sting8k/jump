// Home page: host status, project overview, quick-launch per host.
// Reads shared data from the store (signals).

import { useEffect, useRef, useState } from 'preact/hooks'
import { addProject, removeProject, health, peers, folders, sessions, projects, discovered, launchers as launchersSignal, defaultLauncher as defaultLauncherSignal, launchSession } from './store'
import { PeerLabel } from './peer-label'
import { IconFolder, IconPlay, IconTrash } from './icons'
import type { Folder, LauncherDef } from './types'
import { launchersForPeer } from './launcher'
import {
  buildWorkspaceSuggestions,
  cleanWorkspacePath,
  findWorkspaceSuggestionByPath,
  fsCompletionSuggestions,
  hasProjectPath,
  isWorkspacePath,
  workspaceName,
  type WorkspaceSuggestion,
} from './workspace-suggestions'

/** Strip protocol and trailing slash for display: "https://foo.bar/" → "foo.bar" */
function displayHost(url: string): string {
  return url.replace(/^https?:\/\//, '').replace(/\/+$/, '')
}

export function Home() {
  const healthVal = health.value
  const peersVal = peers.value
  const foldersVal = folders.value
  const sessionsVal = sessions.value
  const localAlive = sessionsVal.filter(s => !s.peer && s.alive).length
  const hostname = healthVal?.hostname ?? 'local'
  const tsUrl = healthVal?.tailscale_url

  const localLaunchers = launchersSignal.value
  const localDefault = defaultLauncherSignal.value
  const peerLaunchers = (peer: string) =>
    launchersForPeer(localLaunchers, localDefault, peersVal, peer).launchers

  return (
    <div class="home">
      {/* ── Hosts ── */}
      <section class="home-hosts">
        <h2 class="home-section-title">Hosts</h2>
        <div class="home-host-grid">
          <HostCard
            name={hostname}
            status="connected"
            url={tsUrl}
            details={[
              healthVal?.version
                ? healthVal.update_available
                  ? `v${healthVal.version} \u2192 v${healthVal.update_available}`
                  : `v${healthVal.version}`
                : undefined,
              `${localAlive} active session${localAlive === 1 ? '' : 's'}`,
            ]}
            launchers={localLaunchers}
          />
          {peersVal.map(p => (
            <HostCard
              key={p.name}
              name={p.name}
              status={p.status}
              url={p.url}
              details={[
                p.version ? `v${p.version}` : undefined,
                p.status === 'connected'
                  ? `${p.session_count} active session${p.session_count === 1 ? '' : 's'}`
                  : p.status === 'offline'
                    ? 'offline'
                    : p.last_error ?? 'disconnected',
              ]}
              launchers={p.status === 'connected' ? peerLaunchers(p.name) : []}
              peer={p.name}
            />
          ))}
        </div>
      </section>

      {/* ── Projects ── */}
      <section class="home-projects">
        <h2 class="home-section-title">Projects</h2>
        <HomeWorkspaceAdd />
        {foldersVal.length > 0 ? (
          <div class="home-project-grid">
            {foldersVal.map(f => <ProjectCard key={f.path} folder={f} />)}
          </div>
        ) : (
          <div class="home-project-empty">No workspaces yet. Add a directory above to pin it here.</div>
        )}
      </section>

      <footer class="home-footer">
        <span class="home-footer-version">Frontend v{__JUMP_VERSION__}</span>
        {healthVal?.version && healthVal.version !== __JUMP_VERSION__ && (
          <button class="home-footer-reload" onClick={() => location.reload()}>
            reload to update
          </button>
        )}
        {healthVal?.update_available && (
          <a
            class="home-footer-update"
            href="https://github.com/sting8k/jump/blob/dev/CHANGELOG.md"
            target="_blank"
          >
            v{healthVal.update_available} available
          </a>
        )}
      </footer>
    </div>
  )
}

function HomeWorkspaceAdd() {
  const inputRef = useRef<HTMLInputElement>(null)
  const [input, setInput] = useState('')
  const [fsSuggestions, setFsSuggestions] = useState<WorkspaceSuggestion[]>([])
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState('')
  const query = input.trim()
  const path = cleanWorkspacePath(input)
  const pathLike = isWorkspacePath(path)
  const duplicate = pathLike && hasProjectPath(projects.value, path)
  const suggestions = query
    ? buildWorkspaceSuggestions({
      fsSuggestions,
      sessionItems: sessions.value,
      configured: projects.value,
      discoveredItems: discovered.value,
      query: input,
    }).slice(0, 8)
    : []
  const topSuggestion = suggestions[0]
  const exactPathSuggestion = pathLike
    ? findWorkspaceSuggestionByPath(suggestions, path)
    : undefined
  const readySuggestion = pathLike ? undefined : topSuggestion
  const canAdd = !adding && query !== '' && !duplicate && (pathLike || Boolean(topSuggestion))

  useEffect(() => {
    if (!pathLike || duplicate) {
      setFsSuggestions([])
      return
    }

    const controller = new AbortController()
    const timer = setTimeout(async () => {
      try {
        const resp = await fetch(`/v1/fs/complete?path=${encodeURIComponent(path)}`, { signal: controller.signal })
        if (!resp.ok) {
          setFsSuggestions([])
          return
        }
        const json = await resp.json()
        setFsSuggestions(fsCompletionSuggestions(json?.data ?? []))
      } catch {
        if (!controller.signal.aborted) setFsSuggestions([])
      }
    }, 180)

    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [path, pathLike, duplicate])

  const handleAdd = async () => {
    const selected = pathLike ? exactPathSuggestion : topSuggestion
    const addPath = pathLike ? path : selected?.path
    if (!addPath) {
      setError('No matching workspace yet. Type a path or a recent project name.')
      return
    }
    if (hasProjectPath(projects.value, addPath)) {
      setError('Workspace already exists')
      return
    }

    setAdding(true)
    setError('')
    try {
      await addProject(selected?.remote ? { remote: selected.remote, paths: [addPath] } : { paths: [addPath] })
      setInput('')
      setFsSuggestions([])
    } finally {
      setAdding(false)
    }
  }

  const selectSuggestion = (suggestionPath: string) => {
    setInput(suggestionPath)
    setError('')
    inputRef.current?.focus()
  }

  return (
    <div class="home-workspace-add">
      <div class="home-workspace-input-row">
        <input
          ref={inputRef}
          class="home-workspace-input"
          type="text"
          placeholder="Add workspace dir, e.g. ~/src/project"
          value={input}
          onInput={e => { setInput((e.target as HTMLInputElement).value); setError('') }}
          onKeyDown={e => {
            if (e.key === 'Tab' && suggestions[0]) {
              e.preventDefault()
              selectSuggestion(suggestions[0].path)
              return
            }
            if (e.key === 'Enter') void handleAdd()
          }}
        />
        <button class="home-workspace-add-btn" disabled={!canAdd} onClick={() => void handleAdd()}>
          {adding ? 'Adding…' : 'Add'}
        </button>
      </div>
      {(pathLike || readySuggestion) && (
        <div class={`home-workspace-preview${duplicate ? ' duplicate' : ''}`}>
          {duplicate ? 'Already added' : `Ready: ${pathLike ? workspaceName(path) : readySuggestion?.name}`}
          <span>{pathLike ? path : readySuggestion?.path}</span>
        </div>
      )}
      {error && <div class="home-workspace-error">{error}</div>}
      {suggestions.length > 0 && (
        <div class="home-workspace-suggestions">
          {suggestions.map(s => (
            <button key={s.path} class="home-workspace-suggestion" onClick={() => selectSuggestion(s.path)}>
              <span>{s.name}</span>
              <small>{s.path}</small>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function ProjectCard({ folder: f }: { folder: Folder }) {
  const alive = f.sessions.filter(s => s.alive).length
  const resumable = f.sessions.filter(s => !s.alive && s.resumable).length
  const handleRemove = () => {
    if (!confirm(`Remove workspace "${f.name}"? Sessions will not be deleted.`)) return
    void removeProject(f.path)
  }

  return (
    <div class="home-project-card">
      <a class="home-project-link" href={`/${f.path}`}>
        <div class="home-project-name"><IconFolder class="home-card-icon" />{f.name}</div>
        <div class="home-project-count">
          {alive > 0 && <span class="home-project-alive">{alive} alive</span>}
          {alive > 0 && resumable > 0 && <span class="home-project-rest"> · </span>}
          {resumable > 0 && <span class="home-project-rest">{resumable} resumable</span>}
          {alive === 0 && resumable === 0 && <span class="home-project-rest">no sessions</span>}
        </div>
      </a>
      <button class="home-project-remove" onClick={handleRemove} title="Remove workspace" aria-label={`Remove workspace ${f.name}`}>
        <IconTrash class="btn-icon" />
      </button>
    </div>
  )
}

function HostCard({
  name, status, url, details, launchers, peer,
}: {
  name: string
  status: string
  url?: string
  details: (string | undefined)[]
  launchers: LauncherDef[]
  peer?: string
}) {
  const [launching, setLaunching] = useState<string | null>(null)
  const linked = status === 'connected' && url

  const handleLaunch = (id: string) => {
    setLaunching(id)
    launchSession(id, peer ? { peer } : undefined).finally(() => setLaunching(null))
  }

  return (
    <div class="home-host-card">
      <div class="home-host-top">
        <span class={`home-host-status ${status}`} />
        {peer && <PeerLabel name={name} />}
        <span class="home-host-name">{name}</span>
      </div>
      <div class="home-host-details">
        {url && (
          linked
            ? <a class="home-host-detail home-host-link" href={url} target="_blank" rel="noopener">{displayHost(url)}</a>
            : <div class="home-host-detail">{displayHost(url)}</div>
        )}
        {details.filter(Boolean).map((d, i) => (
          <div key={i} class="home-host-detail">{d}</div>
        ))}
      </div>
      {launchers.length > 0 && (
        <div class="home-host-launchers">
          {launchers.map(l => (
            <button
              key={l.id}
              class={`home-launch-btn${launching === l.id ? ' launching' : ''}${!l.available ? ' unavailable' : ''}`}
              onClick={() => handleLaunch(l.id)}
              disabled={launching !== null || !l.available}
            >
              <IconPlay class="home-launch-icon" />
              <span>{l.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
