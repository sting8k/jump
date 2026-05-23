import { addPageResumeListener } from './page-resume'

// Presence WebSocket — reports client state to jumpd and receives notification
// commands. The daemon uses this to decide whether, when, and where to show
// OS notifications.

export interface NotifyMessage {
  type: 'notify'
  id: string
  session_id?: string
  title: string
  body: string
  tag: string
  channel?: 'os' | 'in_app'
  navigate_url?: string
}

export interface CancelMessage {
  type: 'cancel'
  id: string
}

export interface ClientState {
  visibility: string
  focused: boolean
  selected_session_id: string | null
  last_interaction: number // Unix seconds
}

export interface PresenceConnection {
  sendState(state: ClientState): void
  sendPermission(permission: string): void
  sendNotificationAck(id: string, action: 'clicked' | 'closed'): void
  close(): void
}

/**
 * Connect to the presence WebSocket. Automatically sends a client-hello on
 * open and routes incoming notify/cancel messages to the provided callbacks.
 *
 * Reconnects automatically on disconnect with exponential backoff, and
 * proactively reconnects when a suspended mobile browser tab resumes.
 */
export function connectPresence(options: {
  onNotify: (msg: NotifyMessage) => void
  onCancel: (msg: CancelMessage) => void
  getNotificationPermission?: () => string
}): PresenceConnection {
  let ws: WebSocket | null = null
  let closed = false
  let backoff = 1000
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  const deviceType = matchMedia('(pointer: coarse)').matches ? 'mobile' : 'desktop'

  // Queue state updates until the socket is ready.
  let pendingState: ClientState | null = null

  function reconnectNow() {
    if (closed) return
    if (reconnectTimer !== null) clearTimeout(reconnectTimer)
    reconnectTimer = null
    ws?.close()
    connect()
  }

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const socket = new WebSocket(`${proto}//${location.host}/v1/presence`)
    ws = socket

    socket.onopen = () => {
      backoff = 1000
      // Read permission fresh on each connect — it may have changed since
      // the previous connection (e.g. user granted permission, then WS reconnected).
      const perm = options.getNotificationPermission?.()
        ?? ('Notification' in window ? Notification.permission : 'unavailable')
      socket.send(JSON.stringify({
        type: 'client-hello',
        device_type: deviceType,
        notification_permission: perm,
      }))
      // Flush any pending state
      if (pendingState) {
        socket.send(JSON.stringify({ type: 'client-state', ...pendingState }))
        pendingState = null
      }
    }

    socket.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        if (msg.type === 'notify') options.onNotify(msg)
        if (msg.type === 'cancel') options.onCancel(msg)
      } catch { /* ignore malformed messages */ }
    }

    socket.onclose = () => {
      if (closed || ws !== socket) return
      reconnectTimer = setTimeout(() => {
        reconnectTimer = null
        if (!closed) connect()
      }, Math.min(backoff, 30000))
      backoff *= 2
    }

    socket.onerror = () => {
      if (ws === socket) socket.close()
    }
  }

  connect()
  const removePageResumeListener = addPageResumeListener(reconnectNow)

  return {
    sendState(state: ClientState) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'client-state', ...state }))
      } else {
        pendingState = state
      }
    },
    sendPermission(permission: string) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'notif-permission', permission }))
      }
    },
    sendNotificationAck(id: string, action: 'clicked' | 'closed') {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'notif-ack', id, action }))
      }
    },
    close() {
      closed = true
      if (reconnectTimer !== null) clearTimeout(reconnectTimer)
      reconnectTimer = null
      removePageResumeListener()
      ws?.close()
    },
  }
}
