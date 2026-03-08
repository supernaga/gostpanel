#!/bin/bash
# GOST Panel 卸载脚本
# 完全卸载 GOST Panel 及其相关文件

set -e

INSTALL_DIR="/opt/gost-panel"
SERVICE_NAME="gost-panel"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${RED}=== GOST Panel Uninstaller ===${NC}"
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Please run as root${NC}"
  exit 1
fi

# 确认卸载
echo -e "${YELLOW}This will completely remove GOST Panel from your system.${NC}"
echo ""
echo "The following will be removed:"
echo "  - Service: $SERVICE_NAME"
echo "  - Install directory: $INSTALL_DIR"
echo "  - Systemd service file: /etc/systemd/system/$SERVICE_NAME.service"
echo ""

read -p "Are you sure you want to continue? (y/N): " confirm
if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
  echo "Uninstallation cancelled."
  exit 0
fi

echo ""

# 停止并禁用服务
echo -e "${YELLOW}Stopping service...${NC}"
if systemctl is-active --quiet $SERVICE_NAME 2>/dev/null; then
  systemctl stop $SERVICE_NAME
  echo -e "${GREEN}Service stopped.${NC}"
else
  echo "Service is not running."
fi

if systemctl is-enabled --quiet $SERVICE_NAME 2>/dev/null; then
  systemctl disable $SERVICE_NAME
  echo -e "${GREEN}Service disabled.${NC}"
fi

# 删除 systemd 服务文件
if [ -f "/etc/systemd/system/$SERVICE_NAME.service" ]; then
  rm -f /etc/systemd/system/$SERVICE_NAME.service
  systemctl daemon-reload
  echo -e "${GREEN}Systemd service file removed.${NC}"
fi

# 询问是否保留数据
echo ""
read -p "Do you want to keep the database? (y/N): " keep_data
if [[ "$keep_data" == "y" || "$keep_data" == "Y" ]]; then
  if [ -d "$INSTALL_DIR/data" ]; then
    BACKUP_DIR="/root/gost-panel-backup-$(date +%Y%m%d%H%M%S)"
    cp -r "$INSTALL_DIR/data" "$BACKUP_DIR"
    echo -e "${GREEN}Database backed up to: $BACKUP_DIR${NC}"
  fi
fi

# 删除安装目录
if [ -d "$INSTALL_DIR" ]; then
  rm -rf "$INSTALL_DIR"
  echo -e "${GREEN}Install directory removed: $INSTALL_DIR${NC}"
else
  echo "Install directory not found: $INSTALL_DIR"
fi

echo ""
echo -e "${GREEN}=== GOST Panel has been uninstalled ===${NC}"
if [[ "$keep_data" == "y" || "$keep_data" == "Y" ]]; then
  echo -e "Database backup: ${YELLOW}$BACKUP_DIR${NC}"
fi
echo ""
