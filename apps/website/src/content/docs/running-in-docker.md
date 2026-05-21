---
title: Running in Docker
description: Run jump in a container with remote access or LAN port mapping.
---

> For native devcontainer integration (auto-discovery, session aggregation), see [Multi-Machine Sessions](/multi-machine#devcontainer-auto-discovery).

There are several ways to access a containerized jump, depending on your network setup. Each is available as a ready-to-run example in the [`examples/`](https://github.com/sting8k/jump/tree/main/examples) directory.

## Tailscale (recommended)

The container registers as its own device on your tailnet. You get HTTPS, cryptographic identity, and access from any device on your tailnet.

**Example:** [`examples/docker-tailscale/`](https://github.com/sting8k/jump/tree/main/examples/docker-tailscale)

```bash
git clone https://github.com/sting8k/jump
cd jump/examples/docker-tailscale

mkdir -p data/{workspace,jump-config,jump-state}
cat > data/jump-config/host.toml << 'EOF'
[tailscale]
enabled = true
hostname = "dev"
EOF

docker compose up -d --build
docker logs dev 2>&1 | grep "login.tailscale.com"
# Visit the URL to register, then open https://dev.your-tailnet.ts.net
```

See [Remote Access](/remote-access/) for Tailscale setup details.

## WireGuard

If you already have a WireGuard tunnel on the host, bind jump to the tunnel interface IP so it's only reachable through the VPN. The tunnel provides encryption; jump provides token authentication.

**Example:** [`examples/docker-wireguard/`](https://github.com/sting8k/jump/tree/main/examples/docker-wireguard)

The compose file binds to your WireGuard IP (e.g. `10.0.0.2:8790:8790`) so jump is only reachable through the tunnel.

## Reverse proxy with OIDC (Traefik + PocketID)

For a full HTTPS setup with OIDC authentication, put Traefik in front of jump with PocketID handling login. Traefik injects the jump bearer token into forwarded requests via a headers middleware, so you only authenticate through your OIDC provider.

**Example:** [`examples/docker-traefik-pocketid/`](https://github.com/sting8k/jump/tree/main/examples/docker-traefik-pocketid)

```
browser → Traefik (HTTPS) → PocketID (OIDC) → jump (HTTP + token)
```

This gives you a valid Let's Encrypt certificate on your own domain, with the jump token as a second layer you never interact with directly. The example uses PocketID but works with any OIDC provider (Authelia, Authentik, Keycloak).

## Setting the auth token

By default, jumpd generates a random auth token on first start. For container deployments where you need a known value (reverse proxy injection, health checks, scripting), there are two options.

**Option 1: Environment variable (recommended for containers).** Set `JUMPD_TOKEN` in your compose file. On first start, jumpd writes it to disk. On subsequent starts, the file already exists and the env var is verified against it.

```bash
openssl rand -hex 32   # copy the output into your compose.yaml or .env
```

```yaml
environment:
  JUMPD_TOKEN: "paste-hex-here"
  JUMPD_LISTEN: "0.0.0.0"
```

**Option 2: Pre-generated file.** Write the token to the state directory before starting the container.

```bash
mkdir -p data/jump-state
openssl rand -hex 32 > data/jump-state/auth-token
chmod 600 data/jump-state/auth-token
```

Either way, any client that needs access can set the `Authorization: Bearer <token>` header. See [Environment variables](/reference/environment/#auth-token) for the full behavior table.

## How it works

The container runs `jumpd` as its entrypoint. Inside the container, `JUMPD_LISTEN=0.0.0.0` binds to all interfaces so the host (or Tailscale) can reach the port. The entrypoint script auto-updates jump binaries on each start.

### Bind address

`JUMPD_LISTEN` controls which address jumpd binds to inside the container and overrides any `listen` value in `host.toml`. The default (`127.0.0.1`) only accepts local connections, which is correct for bare-metal installs but unreachable from outside a container. See [Environment variables](/reference/environment/#bind-address) for details.

### What's blocked over TCP

The `/v1/shutdown` endpoint is blocked on the TCP listener regardless of authentication. Stopping the daemon is a local-only operation available through the Unix socket. This prevents an authenticated network user from killing jumpd.

## Customization

### Adding tools

Edit the `Dockerfile` to add packages, language runtimes, or other tools. Rebuild with:

```bash
docker compose up -d --build
```

### Persistent home

To keep installed tools and shell history across container rebuilds, mount a volume at the home directory:

```yaml
volumes:
  - ./data/home:/root
  - ./data/jump-config:/root/.config/jump
  - ./data/jump-state:/root/.local/state/jump
```

The overlay mounts for jump config and state give the container its own Tailscale identity and hostname, separate from the host.

### Multiple projects

Run separate containers for different projects. Each gets its own Tailscale hostname:

```toml
# data/project-a/jump-config/host.toml
[tailscale]
enabled = true
hostname = "project-a"
```

To see sessions from all containers in a single dashboard instead, set up [Multi-Machine Sessions](/multi-machine) with one jumpd as the hub.
