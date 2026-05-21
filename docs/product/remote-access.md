# Remote Access

## Status

Accepted design contract for future jump remote-access work.

## Scope

jump has one implicit local runtime baseline and two optional remote-access
modes: `tsnet` and `relay`. A missing `[remote]` block means local-only.
The local TCP listener defaults to `127.0.0.1`, but operators may bind it to
private/VPN/container interfaces with `listen` in `host.toml` or the
`JUMPD_LISTEN` environment override; this does not create a third remote-access
mode. Provisioning helpers, SSH tunnels, reverse-proxy snippets, and install
scripts may automate setup, but they are not additional access modes.

```text
jump sessions
  -> jumpd core owns sessions, auth, web UI, API, SSE, and WebSocket handlers
      -> local listener
      -> tsnet listener
      -> relay tunnel agent
```

## Terms

| Term | Meaning |
| --- | --- |
| Runtime core | `jump`, `jumpd`, local PTY sessions, local state, and the shared web/API handler. |
| Local baseline | The implicit browser path to the same-machine `jumpd` handler. It is not configured as a remote mode, even when the TCP bind address is widened for private/VPN/container access. |
| Remote-access mode | An optional non-local transport selected by `[remote].mode`: `tsnet` or `relay`. |
| Provisioning | Optional automation that installs binaries, creates services, configures DNS/TLS, or writes config. |

## Supported Remote-Access Modes

| Mode | Best for | Browser path | Operational tradeoff |
| --- | --- | --- | --- |
| `tsnet` | Private personal/team access when clients are in the tailnet | Browser connects through Tailscale/tsnet to `jumpd` | Requires Tailscale identity, auth keys, and ACL hygiene. |
| `relay` | Public URL access, NAT traversal, or phones/browsers outside the tailnet | Browser connects to public `jump-relayd`; `jumpd` connects outbound by WSS | Requires operating relay infrastructure, TLS, tokens, and relay availability. |

## Design Invariants

- `jumpd` owns session state, scrollback, project/workspace state, auth, and the
  user-facing web/API behavior.
- Remote-access modes only transport traffic to the same `jumpd` handler.
- `jump-relayd` stays stateless about jump product domains. It may authenticate,
  hold tunnels, multiplex HTTP/WebSocket frames, expose health, and report
  whether an agent is connected.
- `jump-relayd` must not persist sessions, cache terminal output, understand
  workspace/session internals, or implement jump business rules.
- Remote mode selection should be explicit and mutually exclusive when remote
  access is enabled. Running more than one remote transport should require an
  explicit advanced/debug decision, not accidental overlapping config.

## Target Configuration Shape

Local access is implicit. Future config should converge on an optional remote
selector that exists only when remote access is enabled:

```toml
[remote]
mode = "relay" # tsnet | relay
public_url = ""

[tailscale]
hostname = "my-jump"
auth_key = ""

[relay]
url = "wss://relay.example.com/_jump/agent"
token = "replace-with-a-shared-secret"
```

Rules:

- Missing `[remote]` means local-only access.
- `remote.mode = "tsnet"` enables the tsnet listener and fails fast when
  required Tailscale config is missing.
- `remote.mode = "relay"` enables the outbound relay agent and fails fast when
  relay URL or token is missing.
- `remote.public_url`, when set, is the browser-facing HTTP/HTTPS URL shown to
  users; it does not replace the relay agent WebSocket URL.
- `tailscale.auth_key`, when set, is passed to tsnet for unattended node login;
  when empty, tsnet uses the interactive login flow.
- Legacy independent `[tailscale].enabled` and `[relay].enabled` fields may be
  migrated gradually, but docs and new management commands should treat
  `[remote].mode` as the source of truth when `[remote]` exists.

## Target Management Commands

Remote management should use direct top-level commands because jump only has two
remote transports:

```bash
jumpd tsnet
jumpd relay
jumpd status
jumpd doctor
```

`jumpd tsnet` should set up or report tsnet state. `jumpd relay` should set
up or report relay state. `jumpd status` should include the selected remote
mode, local URL, remote/public URL when known, connection state, and the last
actionable error.

## Security Boundary

- tsnet mode delegates network reachability to Tailscale identity and ACLs.
- relay mode requires TLS at the public edge and a shared secret or stronger
  agent authentication between `jumpd` and `jump-relayd`.
- The local TCP listener is bearer-token authenticated even when `listen` /
  `JUMPD_LISTEN` exposes it beyond localhost; public internet exposure still
  requires a TLS/proxy/VPN story outside this baseline.
- Browser authentication/authorization remains a `jumpd` responsibility unless a
  future decision explicitly moves part of it to the relay edge.
