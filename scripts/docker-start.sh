#!/bin/bash
set -e

echo "=== GOST Panel Docker 部署 ==="
echo ""

# 检查 Docker
if ! command -v docker &> /dev/null; then
    echo "错误: 请先安装 Docker"
    echo "参考: https://docs.docker.com/get-docker/"
    exit 1
fi

# 检查 Docker Compose
if ! docker compose version &> /dev/null; then
    echo "错误: 请先安装 Docker Compose"
    echo "参考: https://docs.docker.com/compose/install/"
    exit 1
fi

# 切换到项目目录
cd "$(dirname "$0")/.." || exit 1

# 生成 JWT Secret (如果 .env 不存在)
if [ ! -f .env ]; then
    echo "正在生成配置文件 .env ..."

    # 生成随机 JWT Secret
    if command -v openssl &> /dev/null; then
        JWT_SECRET=$(openssl rand -hex 32)
    else
        JWT_SECRET=$(head -c 32 /dev/urandom | xxd -p | tr -d '\n')
    fi

    # 获取当前时间作为 BUILD_TIME
    BUILD_TIME=$(date -u +"%Y-%m-%d %H:%M:%S UTC")

    cat > .env << EOF
# GOST Panel Docker 配置
# 生成时间: ${BUILD_TIME}

# JWT 密钥 (用于身份认证，请妥善保管)
JWT_SECRET=${JWT_SECRET}

# 监听端口 (宿主机端口，容器内始终是 8080)
LISTEN_PORT=8080

# CORS 允许的来源 (多个用逗号分隔，留空则允许所有)
ALLOWED_ORIGINS=

# 调试模式 (生产环境请设置为 false)
DEBUG=false

# 版本信息 (构建时自动设置)
VERSION=dev
BUILD_TIME=${BUILD_TIME}
EOF
    echo "配置文件已生成: .env"
    echo ""
else
    echo "使用现有配置文件: .env"
    echo ""
fi

# 创建数据目录
mkdir -p data
echo "数据目录已创建: ./data"
echo ""

# 构建并启动
echo "正在构建 Docker 镜像..."
echo "提示: 首次构建可能需要 5-10 分钟，请耐心等待"
echo ""

# 从 .env 读取版本信息
source .env

docker compose build \
    --build-arg VERSION="${VERSION:-dev}" \
    --build-arg BUILD_TIME="$(date -u +"%Y-%m-%d %H:%M:%S UTC")"

echo ""
echo "正在启动容器..."
docker compose up -d

echo ""
echo "=== 部署完成 ==="
echo ""
echo "访问地址: http://localhost:${LISTEN_PORT:-8080}"
echo "默认账号: admin"
echo "默认密码: admin123"
echo ""
echo "常用命令:"
echo "  查看日志:   docker compose logs -f"
echo "  查看状态:   docker compose ps"
echo "  停止服务:   docker compose down"
echo "  重启服务:   docker compose restart"
echo "  查看版本:   docker compose exec panel /app/gost-panel -version"
echo "  进入容器:   docker compose exec panel sh"
echo ""
echo "数据文件位置: ./data/panel.db"
echo "配置文件位置: .env"
echo ""
echo "提示: 首次启动请修改默认密码"
