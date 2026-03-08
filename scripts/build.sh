#!/bin/bash
# GOST Panel 构建脚本
# 自动获取 git 版本信息并注入到二进制文件

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 获取版本信息
get_version() {
    # 优先使用 git tag
    if git describe --tags --exact-match 2>/dev/null; then
        return
    fi
    # 否则使用 tag-commits-hash 格式
    if git describe --tags 2>/dev/null; then
        return
    fi
    # 没有 tag 则使用 commit hash
    git rev-parse --short HEAD 2>/dev/null || echo "dev"
}

VERSION=$(get_version)
BUILD_TIME=$(date -u '+%Y-%m-%d %H:%M:%S')
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo -e "${GREEN}Building GOST Panel${NC}"
echo -e "Version: ${YELLOW}${VERSION}${NC}"
echo -e "Build Time: ${YELLOW}${BUILD_TIME}${NC}"
echo -e "Commit: ${YELLOW}${COMMIT}${NC}"
echo ""

# ldflags 定义
LDFLAGS="-X 'github.com/AliceNetworks/gost-panel/internal/api.CurrentAgentVersion=${VERSION}'"
LDFLAGS="${LDFLAGS} -X 'github.com/AliceNetworks/gost-panel/internal/api.AgentBuildTime=${BUILD_TIME}'"

# 项目根目录
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# 构建目标
build_panel() {
    echo -e "${GREEN}Building Panel...${NC}"
    GOMAXPROCS=1 go build -ldflags "${LDFLAGS}" -o gost-panel ./cmd/panel/
    echo -e "${GREEN}✓ Panel built: gost-panel${NC}"
}

build_agent() {
    local os=$1
    local arch=$2
    local output="dist/agents/gost-agent-${os}-${arch}"

    if [ "$os" = "windows" ]; then
        output="${output}.exe"
    fi

    echo -e "${GREEN}Building Agent for ${os}/${arch}...${NC}"

    AGENT_LDFLAGS="-X 'main.AgentVersion=${VERSION}'"
    AGENT_LDFLAGS="${AGENT_LDFLAGS} -X 'main.AgentBuildTime=${BUILD_TIME}'"
    AGENT_LDFLAGS="${AGENT_LDFLAGS} -X 'main.AgentCommit=${COMMIT}'"

    mkdir -p dist/agents
    GOOS=$os GOARCH=$arch go build -ldflags "${AGENT_LDFLAGS}" -o "$output" ./cmd/agent/
    echo -e "${GREEN}✓ Agent built: ${output}${NC}"
}

build_frontend() {
    echo -e "${GREEN}Building Frontend...${NC}"
    cd "$PROJECT_ROOT/web"
    NODE_OPTIONS="--max-old-space-size=1024" npm run build
    cd "$PROJECT_ROOT"
    echo -e "${GREEN}✓ Frontend built${NC}"
}

# 解析参数
case "${1:-all}" in
    panel)
        build_panel
        ;;
    agent)
        # 构建当前平台的 agent
        build_agent "$(go env GOOS)" "$(go env GOARCH)"
        ;;
    agent-all)
        # 构建所有平台的 agent
        build_agent linux amd64
        build_agent linux arm64
        build_agent darwin amd64
        build_agent darwin arm64
        build_agent windows amd64
        ;;
    frontend|web)
        build_frontend
        ;;
    all)
        build_frontend
        build_panel
        ;;
    deploy)
        build_frontend
        build_panel
        echo -e "${GREEN}Deploying...${NC}"
        systemctl stop gost-panel || true
        cp gost-panel /opt/gost-panel/
        systemctl start gost-panel
        echo -e "${GREEN}✓ Deployed and restarted${NC}"
        ;;
    *)
        echo "Usage: $0 {panel|agent|agent-all|frontend|all|deploy}"
        echo ""
        echo "Commands:"
        echo "  panel      - Build panel server only"
        echo "  agent      - Build agent for current platform"
        echo "  agent-all  - Build agent for all platforms (linux, darwin, windows)"
        echo "  frontend   - Build frontend only"
        echo "  all        - Build frontend + panel (default)"
        echo "  deploy     - Build all and deploy to /opt/gost-panel"
        exit 1
        ;;
esac

echo ""
echo -e "${GREEN}Build completed!${NC}"
