.PHONY: all build run dev clean

all: build

# 构建前端
build-web:
	cd web && npm install && npm run build

# 构建后端
build-backend:
	go build -o bin/gost-panel ./cmd/panel
	go build -o bin/gost-agent ./cmd/agent

# 完整构建
build: build-web build-backend

# 开发模式运行后端
run:
	go run ./cmd/panel

# 开发模式 (前后端分离)
dev:
	@echo "Starting backend on :8080..."
	@go run ./cmd/panel &
	@echo "Starting frontend on :5173..."
	@cd web && npm run dev

# 清理
clean:
	rm -rf bin/
	rm -rf web/node_modules
	rm -rf internal/api/dist

# 安装依赖
deps:
	go mod tidy
	cd web && npm install

# Docker 构建
docker-build:
	docker build -t gost-panel .

# Docker 运行
docker-run:
	docker-compose up -d

# 代码质量检查
lint:
	golangci-lint run

# 自动修复代码问题
lint-fix:
	golangci-lint run --fix

# 运行测试
test:
	go test -v -race -coverprofile=coverage.out ./...

# 查看测试覆盖率
coverage:
	go tool cover -html=coverage.out

# 格式化代码
fmt:
	gofmt -s -w .
	goimports -w .

# 安全检查
security:
	gosec ./...

# 完整检查（格式化 + lint + 测试）
check: fmt lint test

# 生成 API 文档（如果使用 swag）
docs:
	swag init -g cmd/panel/main.go -o docs

# 查看项目统计
stats:
	@echo "=== 代码统计 ==="
	@find . -name "*.go" -not -path "./vendor/*" | xargs wc -l | tail -1
	@echo ""
	@echo "=== 文件数量 ==="
	@find . -name "*.go" -not -path "./vendor/*" | wc -l
	@echo ""
	@echo "=== 最大文件 ==="
	@find . -name "*.go" -not -path "./vendor/*" -exec wc -l {} + | sort -rn | head -5
