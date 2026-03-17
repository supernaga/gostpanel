package config

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// DefaultJWTSecret is the insecure default - used only for detection
const DefaultJWTSecret = "gost-panel-secret-change-me"

// 默认配置常量
const (
	DefaultGitHubRawURL = "https://raw.githubusercontent.com/supernaga/gostpanel/main/scripts"
	DefaultGOSTVersion  = "3.0.0-rc10"
)

type Config struct {
	ListenAddr     string   // 面板监听地址
	DBPath         string   // 数据库路径
	JWTSecret      string   // JWT 密钥
	AgentGRPCAddr  string   // Agent gRPC 监听地址
	Debug          bool     // 调试模式
	AllowedOrigins []string // 允许的 CORS 来源
	GitHubRawURL   string   // GitHub Raw 文件 URL
	GOSTVersion    string   // GOST 版本号
}

func Load() *Config {
	jwtSecret := getEnv("JWT_SECRET", "")

	// 如果未设置 JWT_SECRET，尝试从文件加载或生成并持久化
	if jwtSecret == "" || jwtSecret == DefaultJWTSecret {
		if jwtSecret == DefaultJWTSecret {
			log.Println("WARNING: Using default JWT_SECRET is insecure! Please set JWT_SECRET environment variable.")
		}

		// Try to load from persisted file
		dbPath := getEnv("DB_PATH", "./data/panel.db")
		secretFile := filepath.Join(filepath.Dir(dbPath), ".jwt_secret")
		if data, err := os.ReadFile(secretFile); err == nil && len(data) >= 32 {
			jwtSecret = strings.TrimSpace(string(data))
			log.Println("Loaded JWT_SECRET from", secretFile)
		} else {
			// Generate and persist
			randomBytes := make([]byte, 32)
			if _, err := rand.Read(randomBytes); err != nil {
				log.Fatal("FATAL: Failed to generate random JWT secret:", err)
			}
			jwtSecret = hex.EncodeToString(randomBytes)

			// Save to file so it persists across restarts
			if dir := filepath.Dir(secretFile); dir != "" {
				os.MkdirAll(dir, 0700)
			}
			if err := os.WriteFile(secretFile, []byte(jwtSecret), 0600); err != nil {
				log.Println("WARNING: Could not persist JWT_SECRET to file:", err)
				log.Println("WARNING: Tokens will be invalidated on restart. Set JWT_SECRET env var for persistence.")
			} else {
				log.Println("Generated and saved JWT_SECRET to", secretFile)
			}
		}
	}

	// 解析允许的 CORS 来源
	allowedOrigins := parseAllowedOrigins(getEnv("ALLOWED_ORIGINS", ""))

	return &Config{
		ListenAddr:     getEnv("LISTEN_ADDR", ":8080"),
		DBPath:         getEnv("DB_PATH", "./data/panel.db"),
		JWTSecret:      jwtSecret,
		AgentGRPCAddr:  getEnv("AGENT_GRPC_ADDR", ":9090"),
		Debug:          getEnv("DEBUG", "false") == "true",
		AllowedOrigins: allowedOrigins,
		GitHubRawURL:   getEnv("GITHUB_RAW_URL", DefaultGitHubRawURL),
		GOSTVersion:    getEnv("GOST_VERSION", DefaultGOSTVersion),
	}
}

// parseAllowedOrigins 解析允许的 CORS 来源
func parseAllowedOrigins(origins string) []string {
	if origins == "" {
		return nil // nil 表示使用默认策略
	}
	var result []string
	for _, origin := range strings.Split(origins, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			result = append(result, origin)
		}
	}
	return result
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
