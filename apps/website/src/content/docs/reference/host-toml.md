---
title: host.toml
description: Reference for ~/.config/jump/host.toml — daemon behavior.
tableOfContents:
  maxHeadingLevel: 3
---

`~/.config/jump/host.toml` (or `$XDG_CONFIG_HOME/jump/host.toml`)

Daemon behavior. jumpd reads this file once at startup. Create or edit it manually. The only command that modifies this file is `jumpd remote`, which can add the `[tailscale]` section with your confirmation. If the file does not exist, safe defaults are used. Changes require restarting jumpd.

## Example

```toml
# TCP listener. Defaults to localhost:8790.
# Set listen = "0.0.0.0" to accept LAN/VPN/container traffic.
listen = "127.0.0.1"
port = 8790

# Optional Tailscale remote access.
# See the Remote Access guide for setup.
[tailscale]
enabled = false
hostname = "jump"       # → jump.your-tailnet.ts.net
allow = []               # additional login names (owner is auto-whitelisted)

# Optional outbound relay access through jump-relayd.
[relay]
enabled = false
url = "wss://jump.example.com/_jump/agent"
token = "change-me"


# Auto-discover peers. All flags default to true.
[discovery]
tailscale = true         # discover other jump instances on the tailnet
devcontainers = true     # subscribe to Docker events, register jump containers

# Manual peers (remote jumpd instances to aggregate sessions from).
[[peers]]
name = "server"
url = "http://10.0.0.5:8790"
token_file = "~/.config/jump/tokens/server"
```

## Fields

### Top-level

| Field | Type | Default | Range | Description |
|-------|------|---------|-------|-------------|
| `listen` | `string` | `"127.0.0.1"` | loopback, private, link-local, CGNAT, ULA, or unspecified IP | TCP bind address. Use `"0.0.0.0"` for all IPv4 interfaces. |
| `port` | `number` | `8790` | 1–65535 | TCP port for the HTTP listener. |

### `[tailscale]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `boolean` | `false` | Enable Tailscale remote access. |
| `hostname` | `string` | `"jump"` | Tailscale machine name (becomes `<hostname>.your-tailnet.ts.net`). Must be non-empty when enabled. Changing this value automatically clears the Tailscale state and re-registers the device under the new name on the next restart. |
| `allow` | `string[]` | `[]` | Additional Tailscale login names to allow (owner is auto-whitelisted). Each must contain `@`. |

### `[relay]`

> Experimental. Use this when jumpd cannot be reached directly and should connect outbound to a `jump-relayd` server over WebSocket.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | `boolean` | `false` | Connect to a jump relay server. |
| `url` | `string` | `""` | Agent WebSocket URL, e.g. `wss://jump.example.com/_jump/agent`. Required when enabled. |
| `token` | `string` | `""` | Bearer token shared with `jump-relayd -token`. Required when enabled. |


### `[discovery]`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `tailscale` | `boolean` | `true` | Discover other jump instances on the tailnet via `WatchIPNBus`. Only active when `tailscale.enabled` is also true. |
| `devcontainers` | `boolean` | `true` | Subscribe to Docker events and register any container with the jump devcontainer feature as a peer. Skipped if the Docker CLI is not installed. |

### `[[peers]]` (array of tables)

One table per manual peer. Each peer requires `name`, `url`, and exactly one of `token`, `token_file`, `token_command`.

| Field | Type | Description |
|-------|------|-------------|
| `name` | `string` | Unique peer identifier. Appears in URLs (`/@name/`) and session IDs. |
| `url` | `string` | Base URL of the remote jumpd, e.g. `http://host:8790`. |
| `token` | `string` | Inline bearer token. Quick but leaks into your dotfiles. |
| `token_file` | `string` | Path to a file containing the token. Tilde expansion is supported. |
| `token_command` | `string` | Shell command (via `sh -c`) whose stdout is the token. Use for 1Password / pass / op integrations. 10 second timeout. |

## Strict validation

The config file is strictly validated at startup. jumpd refuses to start if:

- **Unknown keys** are present, catching typos like `alow` instead of `allow`
- **`allow` entries don't contain `@`**, likely not a valid Tailscale login name
- **`hostname` is empty** when Tailscale is enabled
- **`relay.url` or `relay.token` are empty** when relay is enabled, or `relay.url` is not `ws://` / `wss://`
- **`listen` is not a valid safe bind IP** (public IPs are rejected; use loopback, private/VPN, or `0.0.0.0` / `::`)
- **`port` is out of range** (must be 1–65535)
- **A `[[peers]]` entry is missing required fields** (`name`, `url`) or specifies more than one token source
- **Two `[[peers]]` entries share the same `name`**
- **TOML syntax is invalid**

This is intentional. Silent fallback to defaults is dangerous for security settings. See [Security](/security) for the reasoning.
