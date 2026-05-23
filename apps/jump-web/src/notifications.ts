export interface NotificationPreferences {
  inApp: boolean
  os: boolean
}

export const DEFAULT_NOTIFICATION_PREFERENCES: NotificationPreferences = {
  inApp: false,
  os: false,
}

export function normalizeNotificationPreferences(value: unknown): NotificationPreferences {
  if (!value || typeof value !== 'object') return DEFAULT_NOTIFICATION_PREFERENCES
  const record = value as Record<string, unknown>
  return {
    inApp: record.in_app === true || record.inApp === true,
    os: record.os === true,
  }
}

export function serializeNotificationPreferences(preferences: NotificationPreferences): { in_app: boolean; os: boolean } {
  const normalized = normalizeNotificationPreferences(preferences)
  return {
    in_app: normalized.inApp,
    os: normalized.os,
  }
}
