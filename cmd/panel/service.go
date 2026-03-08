package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/AliceNetworks/gost-panel/internal/api"
	"github.com/AliceNetworks/gost-panel/internal/config"
	"github.com/AliceNetworks/gost-panel/internal/model"
	"github.com/AliceNetworks/gost-panel/internal/service"
	svc "github.com/kardianos/service"
)

var svcConfig = &svc.Config{
	Name:        "gost-panel",
	DisplayName: "GOST Panel",
	Description: "GOST Proxy Management Panel",
	Option:      makeServiceOptions(),
}

type program struct {
	cancel context.CancelFunc
}

func (p *program) Start(s svc.Service) error {
	go p.run()
	return nil
}

func (p *program) Stop(s svc.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func (p *program) run() {
	// Parse flags (os.Args has been adjusted by handleServiceCommand)
	parseFlags()

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	cfg := config.Load()

	if *listenAddr != "" {
		cfg.ListenAddr = *listenAddr
	}
	if *dbPath != "" {
		cfg.DBPath = *dbPath
	}
	if *debug {
		cfg.Debug = true
	}

	db, err := model.InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}

	svcInst := service.NewService(db, cfg)

	go startTrafficRecorder(svcInst)
	go startSessionCleaner(svcInst)

	server := api.NewServer(svcInst, cfg)

	log.Printf("GOST Panel starting on %s", cfg.ListenAddr)
	if err := server.RunWithContext(ctx); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleServiceCommand() {
	if len(os.Args) < 3 {
		printServiceUsage()
		os.Exit(1)
	}

	action := os.Args[2]

	// Capture extra flags for install (will be baked into the service config)
	var svrArgs []string
	if action == "install" && len(os.Args) > 3 {
		svrArgs = os.Args[3:]
	}

	// Set working directory to the binary's directory
	execPath, _ := os.Executable()
	svcConfig.WorkingDirectory = filepath.Dir(execPath)
	svcConfig.Arguments = append([]string{"service", "run"}, svrArgs...)

	p := &program{}
	svr, err := svc.New(p, svcConfig)
	if err != nil {
		fmt.Printf("Error creating service: %v\n", err)
		os.Exit(1)
	}

	switch action {
	case "install":
		_ = svr.Stop()
		_ = svr.Uninstall()
		if err := svr.Install(); err != nil {
			fmt.Printf("Install failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service installed successfully")
		fmt.Println()
		fmt.Println("Usage:")
		fmt.Println("  gost-panel service start    - Start the service")
		fmt.Println("  gost-panel service stop     - Stop the service")
		fmt.Println("  gost-panel service restart  - Restart the service")
		fmt.Println("  gost-panel service uninstall - Remove the service")

	case "uninstall":
		_ = svr.Stop()
		if err := svr.Uninstall(); err != nil {
			fmt.Printf("Uninstall failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Service uninstalled successfully")

	case "start", "stop", "restart":
		if err := svc.Control(svr, action); err != nil {
			fmt.Printf("%s failed: %v\n", action, err)
			os.Exit(1)
		}
		fmt.Printf("Service %s successfully\n", action)

	case "status":
		status, err := svr.Status()
		if err != nil {
			fmt.Printf("Status check failed: %v\n", err)
			os.Exit(1)
		}
		switch status {
		case svc.StatusRunning:
			fmt.Println("Service is running")
		case svc.StatusStopped:
			fmt.Println("Service is stopped")
		default:
			fmt.Println("Service status unknown")
		}

	case "run":
		// Called by the service manager (systemd/Windows SCM)
		// Adjust os.Args to strip "service" and "run" so flag.Parse works
		os.Args = append(os.Args[:1], os.Args[3:]...)
		_ = svr.Run()

	default:
		printServiceUsage()
		os.Exit(1)
	}
}

func printServiceUsage() {
	fmt.Println("GOST Panel - Service Management")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gost-panel service <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  install    Install as system service")
	fmt.Println("  uninstall  Remove system service")
	fmt.Println("  start      Start the service")
	fmt.Println("  stop       Stop the service")
	fmt.Println("  restart    Restart the service")
	fmt.Println("  status     Check service status")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  gost-panel service install")
	fmt.Println("  gost-panel service install -listen :9000 -db /var/lib/gost-panel/panel.db")
	fmt.Println("  gost-panel service start")
	fmt.Println("  gost-panel service stop")
}
