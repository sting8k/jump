---
title: Environment variables
description: Environment variables used and set by jump.
tableOfContents:
  maxHeadingLevel: 3
---

## jumpd

Variables that affect the daemon.

| Variable | Purpose | Default |
|----------|---------|---------|
| `JUMPD_LISTEN` | Override the TCP bind address from `host.toml` (IPv4 or IPv6). | *(unset)* |
| `JUMPD_TOKEN` | Seed the auth token file on first start. | *(none)* |
| `XDG_CONFIG_HOME` | Base directory for config files. | `~/.config` |
| `XDG_STATE_HOME` | Base directory for runtime state (socket, auth token). | `~/.local/state` |
| `JUMPD_DEV_PROXY` | Proxy frontend requests to a Vite dev server (development only). | *(none)* |

### Bind address

By default jumpd binds to `127.0.0.1` (localhost only). All TCP connections require bearer token authentication.

For persistent host config, set [`listen`](/reference/host-toml/#top-level):

```toml
listen = "0.0.0.0"
```

For systemd, Docker, or one-off runs, `JUMPD_LISTEN` overrides `host.toml`:

```bash
JUMPD_LISTEN=0.0.0.0 jumpd run
```

### Auth token

`JUMPD_TOKEN` seeds the auth token file (`~/.local/state/jump/auth-token`) on first start. This is a provisioning convenience for container deployments where mounting a pre-generated file is impractical.

The value must be at least 64 hex characters (`openssl rand -hex 32` produces exactly this).

**Behavior:**

| Token file | `JUMPD_TOKEN` | Result |
|------------|---------------|--------|
| missing | not set | Generate a random token, write to file |
| missing | set | Validate, write to file |
| present | not set | Use file |
| present | matches env | Use file |
| present | differs | **Refuse to start** |
| corrupted | any | **Refuse to start** |

After reading, jumpd **unsets** `JUMPD_TOKEN` from the process environment so child shells (your terminal sessions) don't inherit it. This reduces but does not eliminate exposure: the original value may still be visible via `/proc/*/environ` or `docker inspect`. The file at `~/.local/state/jump/auth-token` (permissions `0600`) is the primary storage and the safer long-term secret location.

For a known token in Docker Compose:

```bash
openssl rand -hex 32   # copy the output
```

```yaml
environment:
  JUMPD_TOKEN: "paste-hex-here"
  JUMPD_LISTEN: "0.0.0.0"
```

On first start, jumpd writes the token to disk. On subsequent starts, the file already exists and the env var is verified against it.

## jump (CLI)

Variables that affect the session runner.

| Variable | Purpose | Default |
|----------|---------|---------|
| `JUMP_ADAPTER` | Force a specific adapter instead of auto-detection. | *(auto)* |
| `JUMP_SOCKET_DIR` | Directory for per-session Unix sockets. | `/tmp/jump-sessions` |

## Set by jump in child processes

These are available inside every session launched by `jump`. Use them to detect that you are running inside jump, or to communicate back to the session runner.

| Variable | Purpose | Example |
|----------|---------|---------|
| `JUMP` | Always `1` inside a jump session. Used for nested-session detection. | `1` |
| `JUMP_SOCKET` | Unix socket path for callbacks to the session runner. | `/tmp/jump-sessions/sess-abc123.sock` |
| `JUMP_SESSION_ID` | Unique session identifier. | `sess-abc123` |
| `JUMP_ADAPTER` | Name of the matched adapter. | `pi`, `shell` |
| `JUMP_RUNNER_VERSION` | Version of the jump runner hosting the session. | `0.4.0` |

See [Adapter Architecture](/develop/adapter-architecture) for how to use the child-to-runner API.
