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

Sidebar dots and the mobile hamburger badge are state indicators, not notification channels. They remain always on because they mirror session truth (`working`, `error`, `unread`, and transient `activity`). Notification settings only gate delivery channels: in-app toasts, browser OS notifications, and ntfy push.

Session notification channels are user-controlled and default off:
- Sidebar dots and tab title badges remain always-on session state.
- New activity can repulse an unread sidebar dot, but it does not bypass notification settings.
- Selected sessions suppress attention dots (`error`, `working`, `unread`, `active`, `fading`) because the viewed terminal is the foreground state.
- In-app toasts require the in-app notification setting.
- OS notifications require both the OS notification setting and browser permission.
- ntfy push requires the ntfy setting, server URL, and topic ID. Token is optional for authenticated/self-hosted servers.
- ntfy messages default to compact privacy-safe text: `[workspace] session finished` or `[workspace] new output`.

### Trigger conditions

| Event | Condition |
|---|---|
| **Session finished** | `status.working` true → false on a live session |
| **New output** | `unread` false → true |

All delivery channels are skipped when a focused client is viewing the session. If jump is focused elsewhere, the daemon sends an in-app toast instead of escalating to background channels. When jump is not focused, browser OS and ntfy delivery share the normal 5-second grace period.

### Escalation model

1. **In-app dot** (always) — yellow/blue indicator on sidebar and hamburger button
2. **Tab title badge** — `(1) jump` when sessions have unread output
3. **In-app toast** — when enabled and jump is focused but the event belongs to another session
4. **OS notification** — when enabled, after a 5-second grace period while jump is not focused; cancelled if user focuses jump within that window
5. **ntfy push** — when enabled, after the same grace period; sent by the daemon and does not require a browser tab
6. **Cross-device routing** — if the active device is idle (>2 min since last interaction), route to the most recently used other device

### Guardrails

Notification delivery is rate-limited to prevent spam:
- max 3 deliveries per session per 2 minutes
- max 2 deliveries per workspace per 1 minute

These caps apply to delivery channels only. Sidebar dots, mobile badges, and tab badges remain state indicators and are not rate-limited.

### Coalescing

If 3+ sessions trigger notifications within the same grace period window,
the daemon sends a single actionable summary ("5 sessions need attention")
instead of individual notifications. Clicking the summary opens the home view.

## Implementation

- `internal/presence` — presence table tracking connected clients
- `internal/notify` — notification router with grace period, coalescing, device routing, and ntfy publishing
- `apps/jump-web/src/presence.ts` — presence WebSocket client with auto-reconnect
- Settings UI: sidebar-top gear controls in-app, OS, and ntfy channels

## Open items

- Notification sounds (optional)
- Per-session notification preferences (mute noisy sessions)
