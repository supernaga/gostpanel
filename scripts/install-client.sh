#!/bin/bash
# GOST Panel 客户端安装脚本 (Agent 模式)
# 支持: Linux (amd64, arm64, armv7, armv6, mips, mipsle, mips64)
# 用法: curl -fsSL URL | bash -s -- -p PANEL_URL -t TOKEN
#   或: wget -qO- URL | bash -s -- -p PANEL_URL -t TOKEN

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

REPO="AliceNetworks/gost-panel"
PANEL_URL=""
TOKEN=""
INSTALL_DIR="/opt/gost-panel"
FORCE_ARCH=""

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# HTTP 下载 (自动检测 curl/wget)
dl() {
    local url="$1" output="$2"
    if command -v curl &>/dev/null; then
        [ -n "$output" ] && curl -fsSL "$url" -o "$output" || curl -fsSL "$url"
    elif command -v wget &>/dev/null; then
        [ -n "$output" ] && wget -qO "$output" "$url" || wget -qO- "$url"
    else
        log_error "curl and wget not found, please install one of them"
        exit 1
    fi
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        -p|--panel) PANEL_URL="$2"; shift 2 ;;
        -t|--token) TOKEN="$2"; shift 2 ;;
        -a|--arch) FORCE_ARCH="$2"; shift 2 ;;
        -h|--help)
            echo "GOST Panel Client Installer (Agent Mode)"
            echo ""
            echo "Usage: $0 -p <panel_url> -t <token> [-a <arch>]"
            echo ""
            echo "Options:"
            echo "  -p, --panel   Panel URL (e.g., http://panel.example.com:8080)"
            echo "  -t, --token   Client token from panel"
            echo "  -a, --arch    Force architecture (amd64, arm64, armv7, armv6, mips, mipsle)"
            exit 0
            ;;
        *) log_error "Unknown option: $1"; exit 1 ;;
    esac
done

if [[ -z "$PANEL_URL" || -z "$TOKEN" ]]; then
    log_error "Missing required parameters"
    echo "Usage: $0 -p <panel_url> -t <token>"
    exit 1
fi

echo "========================================"
echo "   GOST Panel Client Installer"
echo "   (Agent Mode - Built-in Heartbeat)"
echo "========================================"
echo ""
log_info "Panel: $PANEL_URL"

# 检测系统架构
detect_arch() {
    if [[ -n "$FORCE_ARCH" ]]; then
        echo "$FORCE_ARCH"
        return
    fi

    local arch=$(uname -m)
    local gost_arch=""

    case $arch in
        x86_64|amd64) gost_arch="amd64" ;;
        aarch64|arm64) gost_arch="arm64" ;;
        armv7l|armv7) gost_arch="armv7" ;;
        armv6l|armv6) gost_arch="armv6" ;;
        armv5*) gost_arch="armv5" ;;
        mips)
            if echo -n I | od -to2 | head -n1 | cut -f2 -d" " | cut -c6 | grep -q 1; then
                gost_arch="mipsle"
            else
                gost_arch="mips"
            fi
            ;;
        mips64)
            if echo -n I | od -to2 | head -n1 | cut -f2 -d" " | cut -c6 | grep -q 1; then
                gost_arch="mips64le"
            else
                gost_arch="mips64"
            fi
            ;;
        i386|i686) gost_arch="386" ;;
        *) log_error "Unsupported architecture: $arch"; exit 1 ;;
    esac

    echo "$gost_arch"
}

GOST_ARCH=$(detect_arch)
log_info "Detected architecture: $GOST_ARCH"

# 清理旧的 shell 心跳 (从旧版本升级)
cleanup_old_heartbeat() {
    if [[ -f /etc/gost/heartbeat.sh ]]; then
        log_info "Cleaning up old heartbeat script..."
        # 停止旧的 heartbeat timer
        systemctl stop gost-heartbeat.timer 2>/dev/null || true
        systemctl disable gost-heartbeat.timer 2>/dev/null || true
        rm -f /etc/systemd/system/gost-heartbeat.service
        rm -f /etc/systemd/system/gost-heartbeat.timer
        # 移除 cron
        (crontab -l 2>/dev/null | grep -v "gost/heartbeat") | crontab - 2>/dev/null || true
        rm -f /etc/gost/heartbeat.sh
        systemctl daemon-reload 2>/dev/null || true
        log_info "Old heartbeat cleaned up"
    fi
    # 停止旧的 gost-client 服务 (直接运行 GOST 的旧模式)
    if systemctl is-active gost-client &>/dev/null 2>&1; then
        log_info "Stopping old gost-client service..."
        systemctl stop gost-client 2>/dev/null || true
        systemctl disable gost-client 2>/dev/null || true
        rm -f /etc/systemd/system/gost-client.service
        systemctl daemon-reload 2>/dev/null || true
    fi
}

# 下载 Agent
install_agent() {
    log_info "[1/3] Installing Agent..."

    mkdir -p "$INSTALL_DIR"

    # 删除旧文件
    rm -f "$INSTALL_DIR/gost-agent"

    # 获取最新版本
    local latest_version=$(dl "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$latest_version" ]; then
        latest_version="v1.0.0"
    fi

    # 从 GitHub Releases 下载
    local agent_url="https://github.com/$REPO/releases/download/$latest_version/gost-agent-linux-$GOST_ARCH"

    log_info "Downloading agent from GitHub ($latest_version)..."
    if dl "$agent_url" "$INSTALL_DIR/gost-agent" 2>/dev/null; then
        chmod +x "$INSTALL_DIR/gost-agent"
        log_info "Agent downloaded to $INSTALL_DIR/gost-agent"
        return 0
    fi

    # 回退: 从面板下载
    log_warn "GitHub download failed, trying panel..."
    local panel_agent_url="$PANEL_URL/agent/download/linux/$GOST_ARCH"
    if dl "$panel_agent_url" "$INSTALL_DIR/gost-agent" 2>/dev/null; then
        chmod +x "$INSTALL_DIR/gost-agent"
        log_info "Agent downloaded from panel"
        return 0
    fi

    log_error "Failed to download agent binary"
    exit 1
}

# 安装服务
install_service() {
    log_info "[2/3] Installing service..."

    # 使用 agent 内置 service 管理
    if command -v systemctl &>/dev/null; then
        $INSTALL_DIR/gost-agent service install -panel $PANEL_URL -token $TOKEN -mode client
        $INSTALL_DIR/gost-agent service start
    else
        # 非 systemd 系统: 手动创建服务
        if [[ -f /etc/init.d/rcS ]] || command -v update-rc.d &>/dev/null; then
            # sysvinit
            cat > /etc/init.d/gost-client << EOF
#!/bin/sh
### BEGIN INIT INFO
# Provides:          gost-client
# Required-Start:    \$network \$remote_fs
# Required-Stop:     \$network \$remote_fs
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Description:       GOST Panel Client Agent
### END INIT INFO

DAEMON="$INSTALL_DIR/gost-agent"
DAEMON_ARGS="-panel $PANEL_URL -token $TOKEN -mode client"
PIDFILE="/var/run/gost-client.pid"

start() {
    echo "Starting gost-client..."
    start-stop-daemon --start --background --make-pidfile --pidfile \$PIDFILE --exec \$DAEMON -- \$DAEMON_ARGS
}

stop() {
    echo "Stopping gost-client..."
    start-stop-daemon --stop --pidfile \$PIDFILE
    rm -f \$PIDFILE
}

case "\$1" in
    start) start ;;
    stop) stop ;;
    restart) stop; start ;;
    *) echo "Usage: \$0 {start|stop|restart}"; exit 1 ;;
esac
EOF
            chmod +x /etc/init.d/gost-client
            update-rc.d gost-client defaults 2>/dev/null || true
            /etc/init.d/gost-client start
        elif [[ -f /etc/rc.common ]]; then
            # procd (OpenWrt)
            cat > /etc/init.d/gost-client << EOF
#!/bin/sh /etc/rc.common

START=99
STOP=10
USE_PROCD=1

start_service() {
    procd_open_instance
    procd_set_param command $INSTALL_DIR/gost-agent -panel $PANEL_URL -token $TOKEN -mode client
    procd_set_param respawn
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
            chmod +x /etc/init.d/gost-client
            /etc/init.d/gost-client enable
            /etc/init.d/gost-client start
        else
            # fallback
            mkdir -p "$INSTALL_DIR"
            cat > "$INSTALL_DIR/start-client.sh" << EOF
#!/bin/bash
nohup $INSTALL_DIR/gost-agent -panel $PANEL_URL -token $TOKEN -mode client > /var/log/gost-client.log 2>&1 &
EOF
            chmod +x "$INSTALL_DIR/start-client.sh"
            log_warn "No supported init system found. Run '$INSTALL_DIR/start-client.sh' to start manually."
        fi
    fi
}

# 显示连接信息
show_info() {
    log_info "[3/3] Done!"

    echo ""
    echo "========================================"
    echo "    Installation Complete!"
    echo "========================================"
    echo ""
    echo "Agent Mode Features:"
    echo "  - Built-in heartbeat (every 30s)"
    echo "  - Auto config reload"
    echo "  - Auto GOST download"
    echo "  - Auto uninstall when deleted from panel"
    echo "  - Auto update"
    echo ""

    if command -v systemctl &>/dev/null; then
        echo "Commands:"
        echo "  $INSTALL_DIR/gost-agent service status   - Check status"
        echo "  $INSTALL_DIR/gost-agent service restart  - Restart"
        echo "  $INSTALL_DIR/gost-agent service stop     - Stop"
        echo "  journalctl -u gost-client -f             - View logs"
    else
        echo "Commands:"
        echo "  /etc/init.d/gost-client status  - Check status"
        echo "  /etc/init.d/gost-client restart - Restart"
    fi
}

# 主流程
main() {
    cleanup_old_heartbeat
    install_agent
    install_service
    show_info
}

main
