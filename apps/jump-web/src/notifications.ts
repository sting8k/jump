export interface NtfyPreferences {
  enabled: boolean
  serverUrl: string
  topicId: string
  tokenConfigured: boolean
  token?: string
  clearToken?: boolean
  sendDetails: boolean
}

export interface NotificationPreferences {
  inApp: boolean
  os: boolean
  ntfy: NtfyPreferences
}

export const DEFAULT_NTFY_SERVER_URL = 'https://ntfy.sh'

export const DEFAULT_NOTIFICATION_PREFERENCES: NotificationPreferences = {
  inApp: false,
  os: false,
  ntfy: {
    enabled: false,
    serverUrl: DEFAULT_NTFY_SERVER_URL,
    topicId: '',
    tokenConfigured: false,
    sendDetails: false,
  },
}

export function normalizeNotificationPreferences(value: unknown): NotificationPreferences {
  if (!value || typeof value !== 'object') return DEFAULT_NOTIFICATION_PREFERENCES
  const record = value as Record<string, unknown>
  const ntfy = normalizeNtfyPreferences(record.ntfy)
  return {
    inApp: record.in_app === true || record.inApp === true,
    os: record.os === true,
    ntfy,
  }
}

function normalizeNtfyPreferences(value: unknown): NtfyPreferences {
  if (!value || typeof value !== 'object') return DEFAULT_NOTIFICATION_PREFERENCES.ntfy
  const record = value as Record<string, unknown>
  return {
    enabled: record.enabled === true,
    serverUrl: typeof record.server_url === 'string'
      ? record.server_url
      : typeof record.serverUrl === 'string'
        ? record.serverUrl
        : DEFAULT_NTFY_SERVER_URL,
    topicId: typeof record.topic_id === 'string'
      ? record.topic_id
      : typeof record.topicId === 'string'
        ? record.topicId
        : '',
    tokenConfigured: record.token_configured === true || record.tokenConfigured === true || typeof record.token === 'string' && record.token.length > 0,
    token: typeof record.token === 'string' && record.token.length > 0 ? record.token : undefined,
    clearToken: record.clear_token === true || record.clearToken === true,
    sendDetails: record.send_details === true || record.sendDetails === true,
  }
}

export function serializeNotificationPreferences(preferences: NotificationPreferences): Record<string, unknown> {
  const normalized = normalizeNotificationPreferences(preferences)
  const ntfy: Record<string, unknown> = {
    enabled: normalized.ntfy.enabled,
    server_url: normalized.ntfy.serverUrl,
    topic_id: normalized.ntfy.topicId,
    send_details: normalized.ntfy.sendDetails,
  }
  if (normalized.ntfy.token) ntfy.token = normalized.ntfy.token
  if (normalized.ntfy.clearToken) ntfy.clear_token = true
  return {
    in_app: normalized.inApp,
    os: normalized.os,
    ntfy,
  }
}
