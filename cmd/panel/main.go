package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/supernaga/gost-panel/internal/api"
	"github.com/supernaga/gost-panel/internal/config"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/supernaga/gost-panel/internal/service"
)

var (
	listenAddr  = flag.String("listen", "", "Listen address (e.g., :8080, 0.0.0.0:8080)")
	dbPath      = flag.String("db", "", "Database path")
	debug       = flag.Bool("debug", false, "Enable debug mode")
	showVersion = flag.Bool("version", false, "Show version")
	showHelp    = flag.Bool("help", false, "Show help")
)

func parseFlags() {
	flag.Parse()
}

func main() {
	// Check for service subcommand before flag parsing
	if len(os.Args) > 1 && os.Args[1] == "service" {
		handleServiceCommand()
		return
	}

	parseFlags()

	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	if *showVersion {
		fmt.Printf("GOST Panel %s\n", api.CurrentAgentVersion)
		fmt.Printf("Build Time: %s\n", api.AgentBuildTime)
		os.Exit(0)
	}

	// 加载配置
	cfg := config.Load()

	// 命令行参数覆盖环境变量
	if *listenAddr != "" {
		cfg.ListenAddr = *listenAddr
	}
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}
	if *debug {
		cfg.Debug = true
	}

	// 初始化数据库
	db, err := model.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}

	// 初始化服务
	svc := service.NewService(db, cfg)
	defer svc.Close()

	// 启动流量历史记录定时任务
	go startTrafficRecorder(svc)

	// 启动会话清理定时任务
	go startSessionCleaner(svc)

	// 启动 API 服务
	server := api.NewServer(svc, cfg)

	// 使用带信号处理的优雅关闭
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Printf("GOST Panel starting on %s", cfg.ListenAddr)
	if err := server.RunWithContext(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func printUsage() {
	fmt.Println("GOST Panel - GOST Proxy Management Panel")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gost-panel [options]")
	fmt.Println("  gost-panel service <command> [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -listen string    Listen address (default \":8080\")")
	fmt.Println("                    Examples: :8080, 0.0.0.0:8080, 127.0.0.1:8080")
	fmt.Println("  -db string        Database path (default \"./data/panel.db\")")
	fmt.Println("  -debug            Enable debug mode")
	fmt.Println("  -version          Show version")
	fmt.Println("  -help             Show this help")
	fmt.Println()
	fmt.Println("Service Commands:")
	fmt.Println("  service install    Install as system service")
	fmt.Println("  service uninstall  Remove system service")
	fmt.Println("  service start      Start the service")
	fmt.Println("  service stop       Stop the service")
	fmt.Println("  service restart    Restart the service")
	fmt.Println("  service status     Check service status")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  LISTEN_ADDR       Listen address (same as -listen)")
	fmt.Println("  DB_PATH           Database path (same as -db)")
	fmt.Println("  JWT_SECRET        JWT secret key (required for production)")
	fmt.Println("  DEBUG             Enable debug mode (true/false)")
	fmt.Println("  ALLOWED_ORIGINS   Comma-separated list of allowed CORS origins")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  gost-panel -listen :9000")
	fmt.Println("  gost-panel service install -listen :9000")
	fmt.Println("  gost-panel service start")
	fmt.Println("  LISTEN_ADDR=:9000 JWT_SECRET=mysecret gost-panel")
}

// startTrafficRecorder 启动流量记录定时任务
func startTrafficRecorder(svc *service.Service) {
	// 每分钟记录一次流量数据
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// 立即记录一次
	if err := svc.RecordTrafficHistory(); err != nil {
		log.Printf("Failed to record initial traffic history: %v", err)
	}

	for range ticker.C {
		if err := svc.RecordTrafficHistory(); err != nil {
			log.Printf("Failed to record traffic history: %v", err)
		}
	}
}

// startSessionCleaner 启动会话清理定时任务
func startSessionCleaner(svc *service.Service) {
	// 每小时清理一次过期会话
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		if err := svc.CleanupExpiredSessions(); err != nil {
			log.Printf("Failed to cleanup expired sessions: %v", err)
		}
	}
}
