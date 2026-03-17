<p align="center">
  <img src="web/public/gpl-logo.svg" alt="GOST Panel" width="96" height="96">
</p>

<h1 align="center">GOST Panel</h1>

<p align="center">
  A modern web-based management panel for <a href="https://github.com/go-gost/gost">GOST v3</a> proxy servers
</p>

<p align="center">
  <a href="https://github.com/supernaga/gostpanel/actions/workflows/build.yml"><img src="https://github.com/supernaga/gostpanel/actions/workflows/build.yml/badge.svg" alt="Build"></a>
  <a href="https://github.com/supernaga/gostpanel/releases"><img src="https://img.shields.io/github/v/release/supernaga/gostpanel?display_name=tag&sort=semver" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/supernaga/gostpanel" alt="License"></a>
  <img src="https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/Vue-3-4FC08D?logo=vuedotjs&logoColor=white" alt="Vue 3">
</p>

---

## Features

- **Node & Client Management** - Full lifecycle management of GOST proxy nodes and clients with one-click deployment
- **Port Forwarding** - TCP / UDP / RTCP / RUDP / Relay forwarding rules
- **Tunnels** - Entry/exit node pairs for end-to-end encrypted forwarding
- **Proxy Chains** - Multi-hop sequential proxy chains
- **Node Groups** - Load balancing with round-robin, random, FIFO, hash strategies
- **Traffic Monitoring** - Real-time traffic stats and 30-day historical data with ECharts
- **User Management** - Role-based access control (admin / user / viewer) with 2FA (TOTP)
- **Plans & Quotas** - Traffic quotas, speed limits, and resource limits per user
- **Notifications** - Telegram, Webhook, SMTP integration with customizable alert rules
- **Audit Logging** - Complete operation audit trail with IP and user-agent tracking
- **Agent Architecture** - Lightweight agent binary auto-registers, heartbeats, and hot-reloads config
- **Multi-Protocol** - SOCKS5, HTTP, Shadowsocks, HTTP/2, Relay, DNS, and more
- **Multi-Transport** - TCP, TLS, WebSocket, WSS, QUIC, KCP, gRPC, mTLS, H3
- **Dark / Light Theme** - Toggle between themes on login and in-app
- **i18n** - English and Simplified Chinese

## Quick Start

### One-Click Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/install.sh | bash
```

With custom options:

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/install.sh | \
  GOST_PANEL_REPO="supernaga/gostpanel" \
  GOST_PANEL_VERSION="latest" \
  INITIAL_ADMIN_PASSWORD="your-password" \
  bash
```

| Variable | Default | Description |
|----------|---------|-------------|
| `GOST_PANEL_REPO` | `supernaga/gostpanel` | GitHub repository (`owner/repo`) |
| `GOST_PANEL_BRANCH` | `main` | Source branch (used when building from source) |
| `GOST_PANEL_VERSION` | `latest` | Release tag (e.g. `v1.2.3`) or `latest` |
| `GOST_PANEL_INSTALL_DIR` | `/opt/gost-panel` | Installation directory |
| `INITIAL_ADMIN_PASSWORD` | *(auto-generated)* | Initial admin password |

The installer tries a GitHub Release binary first, falls back to source build, and sets up a `systemd` service.

After install, open:

```
http://<SERVER_IP>:8080
```

### Docker

```bash
# Clone and start
git clone https://github.com/supernaga/gostpanel.git
cd gostpanel

# Edit .env (copy from .env.example)
cp .env.example .env
# Change JWT_SECRET !

docker compose up -d
```

Production mode with resource limits:

```bash
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

Docker image is also published to GHCR on every release:

```bash
docker pull ghcr.io/supernaga/gostpanel:latest
```

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/uninstall.sh | bash
```

## Default Login

- **Username:** `admin`
- **Password:** Set via `INITIAL_ADMIN_PASSWORD`, or auto-generated on first startup. Check service logs:

```bash
journalctl -u gost-panel -n 80 --no-pager
```

## Agent Installation

GOST Panel uses a lightweight agent binary on each node/client that auto-registers with the panel, reports heartbeats, and manages the local GOST process.

### Node Agent

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/install-node.sh | \
  bash -s -- -p https://your-panel-url:8080 -t YOUR_NODE_TOKEN
```

### Client Agent

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/install-client.sh | \
  bash -s -- -p https://your-panel-url:8080 -t YOUR_CLIENT_TOKEN
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/install-node.ps1 | iex
```

> Agents support Linux (amd64, arm64, armv5-7, mips/mipsle/mips64), macOS (amd64, arm64), Windows (amd64, 386, arm64), and FreeBSD (amd64, arm64).

## Service Commands

```bash
systemctl status gost-panel
systemctl restart gost-panel
systemctl stop gost-panel
journalctl -u gost-panel -f
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `JWT_SECRET` | `change-me-in-production` | **Must change in production.** Secret for JWT signing |
| `INITIAL_ADMIN_PASSWORD` | *(auto)* | Initial admin password |
| `LISTEN_PORT` | `8080` | Panel listening port |
| `DB_PATH` | `panel.db` | SQLite database path |
| `ALLOWED_ORIGINS` | *(empty)* | CORS allowed origins, comma-separated |
| `DEBUG` | `false` | Enable debug mode |

## Tech Stack

| Layer | Technologies |
|-------|-------------|
| **Backend** | Go, Gin, GORM, SQLite, JWT, WebSocket, Prometheus |
| **Frontend** | Vue 3, TypeScript, Vite, Pinia, Vue Router, Naive UI, ECharts |
| **Ops** | systemd, Docker, GitHub Actions CI/CD |

## Project Structure

```
.
├── cmd/
│   ├── panel/              # Panel service entry point
│   └── agent/              # Node/client agent binary
├── internal/
│   ├── api/                # HTTP handlers, middleware, embedded frontend
│   ├── config/             # Configuration loading
│   ├── model/              # GORM data models
│   ├── service/            # Business logic
│   ├── notify/             # Telegram / Webhook / SMTP notifications
│   └── gost/               # GOST API client & config generator
├── web/                    # Vue 3 frontend (build output → internal/api/dist)
├── scripts/
│   ├── install.sh          # One-click panel installer
│   ├── uninstall.sh        # One-click uninstaller
│   ├── install-node.sh     # Node agent installer (Linux)
│   ├── install-node.ps1    # Node agent installer (Windows)
│   ├── install-client.sh   # Client agent installer (Linux)
│   ├── install-client.ps1  # Client agent installer (Windows)
│   └── build.sh            # Local build helper
├── .github/workflows/      # CI/CD (dev build + release)
├── Dockerfile              # Multi-stage build (Node 20 + Go)
├── docker-compose.yml      # Local Docker setup
├── docker-compose.prod.yml # Production Docker overrides
├── Makefile                # Build targets
└── go.mod
```

## Local Development

```bash
# Install dependencies
make deps

# Build frontend + backend
make build

# Or run in dev mode (backend :8080, frontend :5173)
make dev
```

> Backend uses `go:embed` to serve the frontend, so build frontend before backend for production builds.

### Useful Make Targets

```bash
make build      # Full build (frontend + backend)
make dev        # Dev mode with hot-reload
make lint       # Run golangci-lint
make test       # Run tests with race detection
make fmt        # Format code
make check      # fmt + lint + test
make docker-run # Build and start via Docker
```

## License

[MIT](LICENSE)
