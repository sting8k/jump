import { useCallback, useEffect, useRef, useState } from 'preact/hooks'
import { DEFAULT_NTFY_SERVER_URL, type NtfyPreferences } from './notifications'
import { notificationPreferences, setNotificationPreferences } from './store'

export type NotifPermission = 'default' | 'granted' | 'denied' | 'unavailable'

function randomTopicID(): string {
  const bytes = new Uint8Array(8)
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(bytes)
  } else {
    for (let i = 0; i < bytes.length; i++) bytes[i] = Math.floor(Math.random() * 256)
  }
  return `jump-${Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')}`
}

export function SettingsModal({
  open,
  onClose,
  notifPermission,
  requestNotifPermission,
}: {
  open: boolean
  onClose: () => void
  notifPermission: NotifPermission
  requestNotifPermission: () => Promise<NotifPermission>
}) {
  const backdropRef = useRef<HTMLDivElement>(null)
  const [tokenInput, setTokenInput] = useState('')
  const prefs = notificationPreferences.value
  const ntfy = prefs.ntfy
  const ntfyTopicReady = ntfy.topicId.trim().length > 0

  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  const handleBackdropClick = useCallback((e: MouseEvent) => {
    if (e.target === backdropRef.current) onClose()
  }, [onClose])

  const setInApp = useCallback((enabled: boolean) => {
    void setNotificationPreferences({ ...notificationPreferences.value, inApp: enabled })
  }, [])

  const setOS = useCallback(async (enabled: boolean) => {
    if (!enabled) {
      void setNotificationPreferences({ ...notificationPreferences.value, os: false })
      return
    }

    const permission = notifPermission === 'granted'
      ? 'granted'
      : await requestNotifPermission()
    if (permission === 'granted') {
      void setNotificationPreferences({ ...notificationPreferences.value, os: true })
    } else {
      void setNotificationPreferences({ ...notificationPreferences.value, os: false })
    }
  }, [notifPermission, requestNotifPermission])

  const setNtfy = useCallback((patch: Partial<NtfyPreferences>) => {
    const current = notificationPreferences.value
    void setNotificationPreferences({
      ...current,
      ntfy: { ...current.ntfy, ...patch },
    })
  }, [])

  const saveToken = useCallback(() => {
    const token = tokenInput.trim()
    if (!token) return
    setNtfy({ token, clearToken: false, tokenConfigured: true })
    setTokenInput('')
  }, [setNtfy, tokenInput])

  if (!open) return null

  const osBlocked = notifPermission === 'denied' || notifPermission === 'unavailable'

  return (
    <div class="modal-backdrop" ref={backdropRef} onClick={handleBackdropClick}>
      <div class="modal-panel settings-modal">
        <div class="modal-header">
          <div class="modal-title">Settings</div>
          <button class="modal-close" onClick={onClose}>&times;</button>
        </div>

        <div class="modal-body settings-body">
          <section class="settings-section">
            <div class="settings-section-label">Session notifications</div>
            <p class="settings-help">
              Sidebar dots stay on always. These controls only change delivery channels.
            </p>

            <label class="settings-toggle-row">
              <span>
                <strong>In-app toasts</strong>
                <small>Show a toast when Jump is focused but another session needs attention.</small>
              </span>
              <input
                type="checkbox"
                checked={prefs.inApp}
                onChange={(e) => setInApp((e.currentTarget as HTMLInputElement).checked)}
              />
            </label>

            <label class="settings-toggle-row">
              <span>
                <strong>OS notifications</strong>
                <small>
                  Browser permission: {notifPermission}{osBlocked ? ' — enable it in browser settings first' : ''}
                </small>
              </span>
              <input
                type="checkbox"
                checked={prefs.os && notifPermission === 'granted'}
                disabled={notifPermission === 'unavailable'}
                onChange={(e) => { void setOS((e.currentTarget as HTMLInputElement).checked) }}
              />
            </label>
          </section>

          <section class="settings-section">
            <div class="settings-section-label">ntfy.sh push</div>
            <p class="settings-help">
              Send daemon-side push notifications to an ntfy topic. The topic ID is the shared secret; keep it random.
            </p>

            <label class="settings-toggle-row">
              <span>
                <strong>Enable ntfy</strong>
                <small>{ntfyTopicReady ? 'Publishes [workspace] messages after the normal grace period.' : 'Topic ID required before enabling.'}</small>
              </span>
              <input
                type="checkbox"
                checked={ntfy.enabled && ntfyTopicReady}
                disabled={!ntfyTopicReady}
                onChange={(e) => setNtfy({ enabled: (e.currentTarget as HTMLInputElement).checked })}
              />
            </label>

            <label class="settings-field-row">
              <span>Server URL</span>
              <input
                type="url"
                value={ntfy.serverUrl || DEFAULT_NTFY_SERVER_URL}
                placeholder={DEFAULT_NTFY_SERVER_URL}
                onInput={(e) => setNtfy({ serverUrl: (e.currentTarget as HTMLInputElement).value })}
              />
            </label>

            <label class="settings-field-row">
              <span>Topic ID</span>
              <div class="settings-inline-field">
                <input
                  value={ntfy.topicId}
                  placeholder="jump-a8f3k2m9"
                  onInput={(e) => setNtfy({ topicId: (e.currentTarget as HTMLInputElement).value, enabled: false })}
                />
                <button type="button" onClick={() => setNtfy({ topicId: randomTopicID(), enabled: false })}>Generate</button>
              </div>
            </label>

            <label class="settings-field-row">
              <span>Auth token <small>optional</small></span>
              <div class="settings-inline-field">
                <input
                  type="password"
                  value={tokenInput}
                  placeholder={ntfy.tokenConfigured ? 'configured' : 'optional bearer token'}
                  onInput={(e) => setTokenInput((e.currentTarget as HTMLInputElement).value)}
                  onBlur={saveToken}
                />
                {ntfy.tokenConfigured && (
                  <button type="button" onClick={() => setNtfy({ clearToken: true, tokenConfigured: false })}>Remove</button>
                )}
              </div>
            </label>

            <label class="settings-toggle-row">
              <span>
                <strong>Send details</strong>
                <small>Include session title/body. Off sends compact messages like [gmux] session finished.</small>
              </span>
              <input
                type="checkbox"
                checked={ntfy.sendDetails}
                onChange={(e) => setNtfy({ sendDetails: (e.currentTarget as HTMLInputElement).checked })}
              />
            </label>
          </section>
        </div>
      </div>
    </div>
  )
}
