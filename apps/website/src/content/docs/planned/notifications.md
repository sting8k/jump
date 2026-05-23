---
title: Notifications
description: Daemon-driven session notifications with presence tracking and smart routing.
---

## How it works

The daemon owns all notification decisions. Browser clients are dumb reporters
that show what the daemon tells them to.

**Browser → Daemon** (via `/v1/presence` WebSocket):
- `client-hello`: device type, notification permission
- `client-state`: visibility, focus, selected session, last interaction timestamp
- `notif-permission`: updated permission after user grant/deny
- `notif-ack`: notification clicked/closed acknowledgement

**Daemon → Browser**:
- `notify`: show an in-app toast or OS notification (title, body, session ID, tag for dedup, optional navigation URL)
- `cancel`: dismiss a notification (e.g. user opened the session on another device)

### Settings

Sidebar dots and the mobile hamburger badge are state indicators, not notification channels. They remain always on because they mirror session truth (`working`, `error`, `unread`, and transient `activity`). Notification settings only gate delivery channels: in-app toasts and OS notifications.

Session notification channels are user-controlled and default off:
- Sidebar dots and tab title badges remain always-on session state.
- New activity can repulse an unread sidebar dot, but it does not bypass notification settings.
- In-app toasts require the in-app notification setting.
- OS notifications require both the OS notification setting and browser permission.

### Trigger conditions

| Event | Condition |
|---|---|
| **Session finished** | `status.working` true → false on a live session |
| **New output** | `unread` false → true |

Both are skipped when a focused client is viewing the session. If jump is focused elsewhere, the daemon sends an in-app toast instead of escalating to an OS notification.

### Escalation model

1. **In-app dot** (always) — yellow/blue indicator on sidebar and hamburger button
2. **Tab title badge** — `(1) jump` when sessions have unread output
3. **In-app toast** — when enabled and jump is focused but the event belongs to another session
4. **OS notification** — when enabled, after a 5-second grace period while jump is not focused; cancelled if user focuses jump within that window
5. **Cross-device routing** — if the active device is idle (>2 min since last interaction), route to the most recently used other device

### Coalescing

If 3+ sessions trigger notifications within the same grace period window,
the daemon sends a single actionable summary ("5 sessions need attention")
instead of individual notifications. Clicking the summary opens the home view.

## Implementation

- `internal/presence` — presence table tracking connected clients
- `internal/notify` — notification router with grace period, coalescing, device routing
- `apps/jump-web/src/presence.ts` — presence WebSocket client with auto-reconnect
- Permission UI: "Enable notifications" button in sidebar footer

## Open items

- Background push when no browser tab is open (see [Mobile Notifications](/planned/mobile-notifications))
- Notification sounds (optional)
- Per-session notification preferences (mute noisy sessions)
