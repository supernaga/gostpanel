# GOST Panel

GOST Panel is a web-based management panel for GOST v3, with API, UI, node/client lifecycle management, notifications, and audit logs.

## Quick Start

### One-Click Install (Recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/install.sh | bash
```

Use custom repo/branch/version:

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/install.sh | \
  GOST_PANEL_REPO="supernaga/gostpanel" \
  GOST_PANEL_BRANCH="main" \
  GOST_PANEL_VERSION="latest" \
  bash
```

Optional environment variables:

- `GOST_PANEL_REPO`: GitHub repository in `owner/repo` format
- `GOST_PANEL_BRANCH`: source branch, default `main`
- `GOST_PANEL_VERSION`: release tag (example `v1.2.3`) or `latest` (default)
- `GOST_PANEL_INSTALL_DIR`: install directory, default `/opt/gost-panel`
- `INITIAL_ADMIN_PASSWORD`: initial admin password (optional)

Install script strategy:

1. Try GitHub Release binary first.
2. If no release asset is available, fall back to source build automatically.
3. Install and start as `systemd` service.

Panel URL after install:

```text
http://<SERVER_IP>:8080
```

### One-Click Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts/uninstall.sh | bash
```

## Default Login

- Username: `admin`
- Password:
  - If `INITIAL_ADMIN_PASSWORD` is set, that value is used.
  - Otherwise, the first startup password is generated and printed in service logs:

```bash
journalctl -u gost-panel -n 80 --no-pager
```

## Service Commands

```bash
systemctl status gost-panel
systemctl restart gost-panel
systemctl stop gost-panel
journalctl -u gost-panel -f
```

## Tech Stack

### Backend

- Go
- Gin (HTTP API)
- GORM + SQLite (default)
- WebSocket
- JWT auth
- Prometheus metrics

### Frontend

- Vue 3
- TypeScript
- Vite
- Pinia
- Vue Router
- Naive UI
- ECharts

### Ops

- systemd
- Shell scripts (install/uninstall/build)
- Docker (optional)

## Project Structure

```text
.
|-- cmd/
|   |-- panel/                 # panel service entrypoint
|   `-- agent/                 # node/client agent entrypoint
|-- internal/
|   |-- api/                   # HTTP handlers, middleware, embedded static assets
|   |-- config/                # config loading
|   |-- model/                 # data models
|   |-- service/               # business services
|   |-- notify/                # telegram/webhook/smtp notifications
|   `-- gost/                  # gost related integrations
|-- web/                       # Vue app (build output: internal/api/dist)
|-- scripts/
|   |-- install.sh             # one-click installer (release + source fallback)
|   |-- uninstall.sh           # one-click uninstaller
|   |-- build.sh               # local build helper
|   |-- install-node.sh        # node installation helper
|   `-- install-client.sh      # client installation helper
|-- Dockerfile
|-- docker-compose.yml
|-- go.mod
`-- README.md
```

## Local Development

```bash
# frontend
cd web
npm install
npm run build
cd ..

# backend
go build -o gost-panel ./cmd/panel
./gost-panel
```

Note: backend uses `go:embed` for static web assets, so build frontend before backend.
