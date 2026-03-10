#!/usr/bin/env bash

set -euo pipefail

# You can override these when running the script:
# GOST_PANEL_REPO=supernaga/gostpanel
# GOST_PANEL_BRANCH=main
# GOST_PANEL_VERSION=v1.2.3 (or "latest")
# GOST_PANEL_INSTALL_DIR=/opt/gost-panel
REPO="${GOST_PANEL_REPO:-supernaga/gostpanel}"
BRANCH="${GOST_PANEL_BRANCH:-main}"
VERSION="${GOST_PANEL_VERSION:-latest}"
INSTALL_DIR="${GOST_PANEL_INSTALL_DIR:-/opt/gost-panel}"
SERVICE_NAME="gost-panel"
ENV_FILE="/etc/sysconfig/${SERVICE_NAME}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_err()  { echo -e "${RED}[ERR ]${NC} $*"; }

require_root() {
  if [ "${EUID}" -ne 0 ]; then
    log_err "Please run as root"
    exit 1
  fi
}

check_disk_space() {
  local required_mb=2048
  local available_mb
  available_mb=$(df /tmp | tail -1 | awk '{print int($4/1024)}')

  if [ "${available_mb}" -lt "${required_mb}" ]; then
    log_err "Insufficient disk space. Required: ${required_mb}MB, Available: ${available_mb}MB"
    log_info "Please free up disk space and try again:"
    log_info "  apt-get clean && apt-get autoremove -y"
    log_info "  rm -rf /tmp/* /var/tmp/*"
    exit 1
  fi

  log_info "Disk space check passed (${available_mb}MB available)"
}

detect_arch() {
  local raw
  raw="$(uname -m)"
  case "${raw}" in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l) ARCH="armv7" ;;
    *)
      log_err "Unsupported architecture: ${raw}"
      exit 1
      ;;
  esac
}

install_build_deps() {
  log_info "Installing build dependencies..."

  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y ca-certificates curl tar git build-essential pkg-config nodejs npm golang-go
    return
  fi

  if command -v dnf >/dev/null 2>&1; then
    dnf install -y ca-certificates curl tar git gcc gcc-c++ make nodejs npm golang
    return
  fi

  if command -v yum >/dev/null 2>&1; then
    yum install -y ca-certificates curl tar git gcc gcc-c++ make nodejs npm golang
    return
  fi

  if command -v apk >/dev/null 2>&1; then
    apk add --no-cache bash ca-certificates curl tar git build-base nodejs npm go
    return
  fi

  log_err "Unsupported package manager. Please install Node.js, npm and Go manually."
  exit 1
}

download_release_binary() {
  local release_tag
  local api
  local url

  if [ "${VERSION}" = "latest" ]; then
    api="https://api.github.com/repos/${REPO}/releases/latest"
    release_tag="$(curl -fsSL "${api}" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/' | head -n1 || true)"
  else
    release_tag="${VERSION}"
  fi

  if [ -z "${release_tag}" ]; then
    log_warn "No release tag found from GitHub API"
    return 1
  fi

  url="https://github.com/${REPO}/releases/download/${release_tag}/gost-panel-linux-${ARCH}.tar.gz"
  log_info "Trying release asset: ${url}"

  if ! curl -fsSLI "${url}" >/dev/null 2>&1; then
    log_warn "Release asset not found for ${release_tag} (${ARCH})"
    return 1
  fi

  local tmp_tgz="/tmp/gost-panel-${release_tag}-${ARCH}.tar.gz"
  rm -f "${tmp_tgz}"
  curl -fsSL -o "${tmp_tgz}" "${url}"
  tar -xzf "${tmp_tgz}" -C /tmp
  mv "/tmp/gost-panel-linux-${ARCH}" "${INSTALL_DIR}/gost-panel"
  chmod +x "${INSTALL_DIR}/gost-panel"
  rm -f "${tmp_tgz}"

  log_info "Installed binary from release: ${release_tag}"
  return 0
}

build_from_source() {
  log_warn "Falling back to source build (this may take 5-10 minutes)"
  check_disk_space
  install_build_deps

  local ref_path
  local src_url
  local src_tar="/tmp/gost-panel-src.tar.gz"
  local src_dir="/tmp/gost-panel-src-$$"

  if [ "${VERSION}" = "latest" ]; then
    ref_path="heads/${BRANCH}"
  else
    ref_path="tags/${VERSION}"
  fi

  src_url="https://github.com/${REPO}/archive/refs/${ref_path}.tar.gz"
  log_info "Downloading source: ${src_url}"

  rm -rf "${src_dir}" "${src_tar}"
  mkdir -p "${src_dir}"
  curl -fsSL -o "${src_tar}" "${src_url}"
  tar -xzf "${src_tar}" -C "${src_dir}" --strip-components=1

  log_info "Building frontend (this may take a few minutes)..."
  (
    cd "${src_dir}/web"
    if [ -f "package-lock.json" ]; then
      npm ci --quiet
    else
      npm install --quiet
    fi
    NODE_OPTIONS="--max-old-space-size=1024" npm run build
  )

  log_info "Building panel binary..."
  (
    cd "${src_dir}"
    GOMAXPROCS=1 go build -ldflags="-s -w" -o "${INSTALL_DIR}/gost-panel" ./cmd/panel
  )

  chmod +x "${INSTALL_DIR}/gost-panel"
  rm -rf "${src_dir}" "${src_tar}"
  log_info "Installed binary from source"
}

write_env_file_if_missing() {
  if [ -f "${ENV_FILE}" ]; then
    log_info "Environment file exists, keeping current config: ${ENV_FILE}"
    return
  fi

  mkdir -p /etc/sysconfig
  local jwt_secret
  jwt_secret="$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p)"

  {
    echo "DB_PATH=${INSTALL_DIR}/data/panel.db"
    echo "LISTEN_ADDR=:8080"
    echo "JWT_SECRET=${jwt_secret}"
    if [ -n "${INITIAL_ADMIN_PASSWORD:-}" ]; then
      echo "INITIAL_ADMIN_PASSWORD=${INITIAL_ADMIN_PASSWORD}"
    fi
  } > "${ENV_FILE}"

  chmod 600 "${ENV_FILE}"
  log_info "Created environment file: ${ENV_FILE}"
}

install_or_restart_service() {
  if [ -x "${INSTALL_DIR}/gost-panel" ]; then
    "${INSTALL_DIR}/gost-panel" service stop >/dev/null 2>&1 || true
    "${INSTALL_DIR}/gost-panel" service uninstall >/dev/null 2>&1 || true
  fi

  "${INSTALL_DIR}/gost-panel" service install
  "${INSTALL_DIR}/gost-panel" service start
  sleep 2
}

print_summary() {
  local ip
  ip="$(hostname -I | awk '{print $1}')"
  echo ""
  echo -e "${GREEN}========================================${NC}"
  echo -e "${GREEN}  GOST Panel Installation Complete!${NC}"
  echo -e "${GREEN}========================================${NC}"
  echo ""
  echo -e "📍 Panel URL: ${GREEN}http://${ip}:8080${NC}"
  echo -e "🔧 Service: ${SERVICE_NAME}"
  echo -e "⚙️  Config: ${ENV_FILE}"
  echo ""
  echo -e "${YELLOW}📝 Default Login:${NC}"
  echo -e "   Username: ${GREEN}admin${NC}"
  if [ -n "${INITIAL_ADMIN_PASSWORD:-}" ]; then
    echo -e "   Password: ${GREEN}${INITIAL_ADMIN_PASSWORD}${NC}"
  else
    echo -e "   Password: ${YELLOW}Check service logs:${NC}"
    echo -e "   ${GREEN}journalctl -u ${SERVICE_NAME} -n 80 --no-pager | grep -i password${NC}"
  fi
  echo ""
  echo -e "${YELLOW}🔍 Service Commands:${NC}"
  echo "   systemctl status ${SERVICE_NAME}"
  echo "   systemctl restart ${SERVICE_NAME}"
  echo "   journalctl -u ${SERVICE_NAME} -f"
  echo ""
  echo -e "${YELLOW}🔄 Upgrade:${NC}"
  echo "   curl -fsSL https://raw.githubusercontent.com/${REPO}/${BRANCH}/scripts/install.sh | bash"
  echo ""
}

main() {
  echo -e "${GREEN}=== GOST Panel Installer ===${NC}"
  require_root
  detect_arch

  log_info "Repo: ${REPO}"
  log_info "Branch: ${BRANCH}"
  log_info "Version: ${VERSION}"
  log_info "Arch: ${ARCH}"
  log_info "Install dir: ${INSTALL_DIR}"

  mkdir -p "${INSTALL_DIR}/data"

  if ! download_release_binary; then
    build_from_source
  fi

  write_env_file_if_missing
  install_or_restart_service
  print_summary
}

main "$@"
