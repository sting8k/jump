import { useCallback, useEffect, useRef } from 'preact/hooks'
import { notificationPreferences, setNotificationPreferences } from './store'

export type NotifPermission = 'default' | 'granted' | 'denied' | 'unavailable'

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
  const prefs = notificationPreferences.value

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
              Sidebar dots stay on always. These toggles only control toast and OS notification channels.
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
        </div>
      </div>
    </div>
  )
}
