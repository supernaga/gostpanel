package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/supernaga/gost-panel/internal/config"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/supernaga/gost-panel/internal/notify"
	"github.com/supernaga/gost-panel/internal/service"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

//go:embed all:dist
var staticFS embed.FS

type Server struct {
	svc          *service.Service
	cfg          *config.Config
	router       *gin.Engine
	loginLimiter *RateLimiter
	audit        *AuditLogger
	wsHub        *WSHub
	// API rate limiters
	globalAPILimiter *APIRateLimiter
	writeAPILimiter  *APIRateLimiter
	agentLimiter     *APIRateLimiter
}

func NewServer(svc *service.Service, cfg *config.Config) *Server {
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// 配置 Gin 路由行为，禁用自动重定向以精确匹配路由（避免影响前端 SPA 路由）
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	if len(cfg.AllowedOrigins) > 0 {
		corsConfig.AllowOrigins = cfg.AllowedOrigins
	} else if cfg.Debug {
		// 调试模式：允许 localhost 开发环境跨域
		corsConfig.AllowOriginFunc = func(origin string) bool {
			return strings.HasPrefix(origin, "http://localhost") ||
				strings.HasPrefix(origin, "http://127.0.0.1") ||
				strings.HasPrefix(origin, "https://localhost") ||
				strings.HasPrefix(origin, "https://127.0.0.1")
		}
	} else {
		// 生产环境：未配置 ALLOWED_ORIGINS 时拒绝所有跨域请求
		log.Println("WARNING: No ALLOWED_ORIGINS configured. Cross-origin requests will be rejected.")
		corsConfig.AllowOriginFunc = func(origin string) bool {
			// 生产环境未配置允许源，拒绝跨域请求
			return false
		}
	}

	r.Use(cors.New(corsConfig))

	SetWSOrigins(cfg.AllowedOrigins, cfg.Debug)

	s := &Server{
		svc:              svc,
		cfg:              cfg,
		router:           r,
		loginLimiter:     NewRateLimiter(5, time.Minute, 5*time.Minute), // 登录限流：每分钟最多5次，锁定5分钟
		audit:            NewAuditLogger(svc),
		wsHub:            NewWSHub(),
		globalAPILimiter: NewAPIRateLimiter(200, time.Minute),
		writeAPILimiter:  NewAPIRateLimiter(30, time.Minute),
		agentLimiter:     NewAPIRateLimiter(60, time.Minute),
	}

	// 当 IP 被限流封锁时记录安全日志
	s.loginLimiter.SetOnBlockCallback(func(ip string, attempts int) {
		s.svc.LogOperation(0, "system", "security", "ip_block", 0,
			fmt.Sprintf("IP blocked due to excessive login attempts: %s (%d attempts)", ip, attempts),
			ip, "rate_limiter", "success")
	})

	// Start WebSocket hub
	go s.wsHub.Run()

	s.svc.InitDefaultSiteConfigs()

	s.setupRoutes()

	return s
}

func (s *Server) setupRoutes() {
	s.router.Use(PrometheusMiddleware())

	// Prometheus 监控指标（需要认证）
	s.router.GET("/metrics", s.authMiddleware(), MetricsHandler())

	// API 路由组
	api := s.router.Group("/api")
	{
		// 健康检查（无需认证）
		api.GET("/health", s.healthCheck)

		// 认证相关接口（带限流）
		api.POST("/login", RateLimitMiddleware(s.loginLimiter), s.login)
		api.POST("/login/2fa", RateLimitMiddleware(s.loginLimiter), s.login2FA)
		api.GET("/site-config", s.getPublicSiteConfig)

		// 用户注册相关接口
		api.POST("/register", RateLimitMiddleware(s.loginLimiter), s.register)
		api.POST("/verify-email", s.verifyEmail)
		api.POST("/forgot-password", RateLimitMiddleware(s.loginLimiter), s.forgotPassword)
		api.POST("/reset-password", s.resetPassword)
		api.GET("/registration-status", s.getRegistrationStatus)

		// 以下接口需要 JWT 认证
		auth := api.Group("")
		auth.Use(s.authMiddleware())
		auth.Use(APIRateLimitMiddleware(s.globalAPILimiter)) // 全局 API 限流
		auth.Use(s.viewerWriteBlockMiddleware())
		{
			// 系统统计
			auth.GET("/stats", s.getStats)

			// 全局搜索
			auth.GET("/search", s.globalSearch)

			// 会话管理
			auth.GET("/sessions", s.getSessions)
			auth.DELETE("/sessions/:id", s.deleteSession)
			auth.DELETE("/sessions/others", s.deleteOtherSessions)

			auth.GET("/nodes", s.listNodes)
			auth.GET("/nodes/paginated", s.listNodesPaginated)
			auth.POST("/nodes", APIRateLimitMiddleware(s.writeAPILimiter), s.createNode)
			auth.GET("/nodes/:id", s.getNode)
			auth.PUT("/nodes/:id", APIRateLimitMiddleware(s.writeAPILimiter), s.updateNode)
			auth.DELETE("/nodes/:id", APIRateLimitMiddleware(s.writeAPILimiter), s.deleteNode)
			auth.POST("/nodes/:id/apply", APIRateLimitMiddleware(s.writeAPILimiter), s.applyNodeConfig)
			auth.POST("/nodes/:id/clone", APIRateLimitMiddleware(s.writeAPILimiter), s.cloneNode)
			auth.POST("/nodes/:id/sync", APIRateLimitMiddleware(s.writeAPILimiter), s.syncNodeConfig)
			auth.GET("/nodes/:id/gost-config", s.getNodeGostConfig)
			auth.GET("/nodes/:id/proxy-uri", s.getNodeProxyURI)
			auth.GET("/nodes/:id/install-script", s.getNodeInstallScript)
			auth.GET("/nodes/:id/ping", s.pingNode)
			auth.GET("/nodes/ping", s.pingAllNodes)
			auth.GET("/nodes/:id/health-logs", s.getNodeHealthLogs)
			auth.GET("/health-summary", s.getHealthSummary)

			auth.GET("/nodes/:id/config-versions", s.getConfigVersions)
			auth.POST("/nodes/:id/config-versions", s.createConfigVersion)
			auth.GET("/config-versions/:versionId", s.getConfigVersion)
			auth.POST("/config-versions/:versionId/restore", s.restoreConfigVersion)
			auth.DELETE("/config-versions/:versionId", s.deleteConfigVersion)

			auth.POST("/nodes/batch-enable", s.batchEnableNodes)
			auth.POST("/nodes/batch-disable", s.batchDisableNodes)
			auth.POST("/nodes/batch-delete", s.batchDeleteNodes)
			auth.POST("/nodes/batch-sync", s.batchSyncNodes)

			auth.GET("/clients", s.listClients)
			auth.GET("/clients/paginated", s.listClientsPaginated)
			auth.POST("/clients", s.createClient)
			auth.GET("/clients/:id", s.getClient)
			auth.PUT("/clients/:id", s.updateClient)
			auth.DELETE("/clients/:id", s.deleteClient)
			auth.GET("/clients/:id/install-script", s.getClientInstallScript)
			auth.GET("/clients/:id/gost-config", s.getClientGostConfig)
			auth.GET("/clients/:id/proxy-uri", s.getClientProxyURI)
			auth.POST("/clients/:id/clone", s.cloneClient)

			auth.POST("/clients/batch-enable", s.batchEnableClients)
			auth.POST("/clients/batch-disable", s.batchDisableClients)
			auth.POST("/clients/batch-delete", s.batchDeleteClients)
			auth.POST("/clients/batch-sync", s.batchSyncClients)

			auth.GET("/users", s.listUsers)
			auth.POST("/users", s.createUser)
			auth.GET("/users/:id", s.getUser)
			auth.PUT("/users/:id", s.updateUser)
			auth.DELETE("/users/:id", s.deleteUser)
			auth.POST("/change-password", s.changePassword)

			auth.GET("/profile", s.getProfile)
			auth.PUT("/profile", s.updateProfile)

			auth.POST("/profile/2fa/enable", s.enable2FA)
			auth.POST("/profile/2fa/verify", s.verify2FA)
			auth.POST("/profile/2fa/disable", s.disable2FA)

			auth.GET("/traffic-history", s.getTrafficHistory)

			auth.GET("/notify-channels", s.listNotifyChannels)
			auth.POST("/notify-channels", s.createNotifyChannel)
			auth.GET("/notify-channels/:id", s.getNotifyChannel)
			auth.PUT("/notify-channels/:id", s.updateNotifyChannel)
			auth.DELETE("/notify-channels/:id", s.deleteNotifyChannel)
			auth.POST("/notify-channels/:id/test", s.testNotifyChannel)

			auth.GET("/alert-rules", s.listAlertRules)
			auth.POST("/alert-rules", s.createAlertRule)
			auth.GET("/alert-rules/:id", s.getAlertRule)
			auth.PUT("/alert-rules/:id", s.updateAlertRule)
			auth.DELETE("/alert-rules/:id", s.deleteAlertRule)

			auth.GET("/alert-logs", s.getAlertLogs)

			auth.GET("/operation-logs", s.getOperationLogs)

			auth.GET("/export", s.exportData)
			auth.POST("/import", s.importData)

			auth.GET("/backup", s.backupDatabase)
			auth.POST("/restore", s.restoreDatabase)

			auth.GET("/port-forwards", s.listPortForwards)
			auth.POST("/port-forwards", s.createPortForward)
			auth.GET("/port-forwards/:id", s.getPortForward)
			auth.PUT("/port-forwards/:id", s.updatePortForward)
			auth.DELETE("/port-forwards/:id", s.deletePortForward)
			auth.POST("/port-forwards/:id/clone", s.clonePortForward)

			auth.GET("/node-groups", s.listNodeGroups)
			auth.POST("/node-groups", s.createNodeGroup)
			auth.GET("/node-groups/:id", s.getNodeGroup)
			auth.PUT("/node-groups/:id", s.updateNodeGroup)
			auth.DELETE("/node-groups/:id", s.deleteNodeGroup)
			auth.GET("/node-groups/:id/members", s.listNodeGroupMembers)
			auth.POST("/node-groups/:id/members", s.addNodeGroupMember)
			auth.DELETE("/node-groups/:id/members/:memberId", s.removeNodeGroupMember)
			auth.GET("/node-groups/:id/config", s.getNodeGroupConfig)
			auth.POST("/node-groups/:id/clone", s.cloneNodeGroup)

			auth.GET("/proxy-chains", s.listProxyChains)
			auth.POST("/proxy-chains", s.createProxyChain)
			auth.GET("/proxy-chains/:id", s.getProxyChain)
			auth.PUT("/proxy-chains/:id", s.updateProxyChain)
			auth.DELETE("/proxy-chains/:id", s.deleteProxyChain)
			auth.GET("/proxy-chains/:id/hops", s.listProxyChainHops)
			auth.POST("/proxy-chains/:id/hops", s.addProxyChainHop)
			auth.PUT("/proxy-chains/:id/hops/:hopId", s.updateProxyChainHop)
			auth.DELETE("/proxy-chains/:id/hops/:hopId", s.removeProxyChainHop)
			auth.GET("/proxy-chains/:id/config", s.getProxyChainConfig)
			auth.POST("/proxy-chains/:id/clone", s.cloneProxyChain)

			auth.GET("/tunnels", s.listTunnels)
			auth.POST("/tunnels", s.createTunnel)
			auth.GET("/tunnels/:id", s.getTunnel)
			auth.PUT("/tunnels/:id", s.updateTunnel)
			auth.DELETE("/tunnels/:id", s.deleteTunnel)
			auth.POST("/tunnels/:id/sync", s.syncTunnel)
			auth.GET("/tunnels/:id/entry-config", s.getTunnelEntryConfig)
			auth.GET("/tunnels/:id/exit-config", s.getTunnelExitConfig)
			auth.POST("/tunnels/:id/clone", s.cloneTunnel)

			auth.GET("/templates", s.listTemplates)
			auth.GET("/templates/categories", s.getTemplateCategories)
			auth.GET("/templates/:id", s.getTemplate)

			auth.GET("/client-templates", s.listClientTemplates)
			auth.GET("/client-templates/categories", s.getClientTemplateCategories)
			auth.GET("/client-templates/:id", s.getClientTemplate)

			auth.GET("/site-configs", s.getSiteConfigs)
			auth.PUT("/site-configs", s.updateSiteConfigs)

			auth.GET("/tags", s.listTags)
			auth.GET("/tags/:id", s.getTag)
			auth.POST("/tags", s.createTag)
			auth.PUT("/tags/:id", s.updateTag)
			auth.DELETE("/tags/:id", s.deleteTag)
			auth.GET("/tags/:id/nodes", s.getNodesByTag)

			auth.GET("/nodes/:id/tags", s.getNodeTags)
			auth.POST("/nodes/:id/tags", s.addNodeTag)
			auth.PUT("/nodes/:id/tags", s.setNodeTags)
			auth.DELETE("/nodes/:id/tags/:tagId", s.removeNodeTag)

			auth.POST("/users/:id/verify-email", s.adminVerifyUserEmail)
			auth.POST("/users/:id/resend-verification", s.resendVerificationEmail)
			auth.POST("/users/:id/reset-quota", s.resetUserQuota)
			auth.POST("/users/:id/assign-plan", s.assignUserPlan)
			auth.POST("/users/:id/remove-plan", s.removeUserPlan)
			auth.POST("/users/:id/renew-plan", s.renewUserPlan)

			auth.GET("/plans", s.listPlans)
			auth.GET("/plans/:id", s.getPlan)
			auth.POST("/plans", s.createPlan)
			auth.PUT("/plans/:id", s.updatePlan)
			auth.DELETE("/plans/:id", s.deletePlan)
			auth.GET("/plans/:id/resources", s.getPlanResources)
			auth.PUT("/plans/:id/resources", s.setPlanResources)

			auth.GET("/bypasses", s.listBypasses)
			auth.GET("/bypasses/:id", s.getBypass)
			auth.POST("/bypasses", s.createBypass)
			auth.PUT("/bypasses/:id", s.updateBypass)
			auth.DELETE("/bypasses/:id", s.deleteBypass)
			auth.POST("/bypasses/:id/clone", s.cloneBypass)

			auth.GET("/admissions", s.listAdmissions)
			auth.GET("/admissions/:id", s.getAdmission)
			auth.POST("/admissions", s.createAdmission)
			auth.PUT("/admissions/:id", s.updateAdmission)
			auth.DELETE("/admissions/:id", s.deleteAdmission)
			auth.POST("/admissions/:id/clone", s.cloneAdmission)

			auth.GET("/host-mappings", s.listHostMappings)
			auth.GET("/host-mappings/:id", s.getHostMapping)
			auth.POST("/host-mappings", s.createHostMapping)
			auth.PUT("/host-mappings/:id", s.updateHostMapping)
			auth.DELETE("/host-mappings/:id", s.deleteHostMapping)
			auth.POST("/host-mappings/:id/clone", s.cloneHostMapping)

			auth.GET("/ingresses", s.listIngresses)
			auth.GET("/ingresses/:id", s.getIngress)
			auth.POST("/ingresses", s.createIngress)
			auth.PUT("/ingresses/:id", s.updateIngress)
			auth.DELETE("/ingresses/:id", s.deleteIngress)
			auth.POST("/ingresses/:id/clone", s.cloneIngress)

			auth.GET("/recorders", s.listRecorders)
			auth.GET("/recorders/:id", s.getRecorder)
			auth.POST("/recorders", s.createRecorder)
			auth.PUT("/recorders/:id", s.updateRecorder)
			auth.DELETE("/recorders/:id", s.deleteRecorder)
			auth.POST("/recorders/:id/clone", s.cloneRecorder)

			auth.GET("/routers", s.listRouters)
			auth.GET("/routers/:id", s.getRouter)
			auth.POST("/routers", s.createRouter)
			auth.PUT("/routers/:id", s.updateRouter)
			auth.DELETE("/routers/:id", s.deleteRouter)
			auth.POST("/routers/:id/clone", s.cloneRouter)

			auth.GET("/sds", s.listSDs)
			auth.GET("/sds/:id", s.getSD)
			auth.POST("/sds", s.createSD)
			auth.PUT("/sds/:id", s.updateSD)
			auth.DELETE("/sds/:id", s.deleteSD)
			auth.POST("/sds/:id/clone", s.cloneSD)
		}
	}

	agent := s.router.Group("/agent")
	agent.Use(APIRateLimitMiddleware(s.agentLimiter))
	{
		agent.POST("/register", s.agentRegister)
		agent.POST("/heartbeat", s.agentHeartbeat)
		agent.GET("/config/:token", s.agentGetConfig)
		agent.GET("/version", s.agentGetVersion)
		agent.GET("/check-update", s.agentCheckUpdate)
		agent.GET("/download/:os/:arch", s.agentDownload)
		agent.POST("/client-heartbeat/:token", s.clientHeartbeat)
	}

	s.router.GET("/ws", s.wsAuthMiddleware(), s.handleWebSocket)

	scripts := s.router.Group("/scripts")
	{
		scripts.GET("/install-node.sh", s.serveInstallScript("install-node.sh"))
		scripts.GET("/install-client.sh", s.serveInstallScript("install-client.sh"))
		scripts.GET("/install-node.ps1", s.serveInstallScript("install-node.ps1"))
		scripts.GET("/install-client.ps1", s.serveInstallScript("install-client.ps1"))
		scripts.GET("/client/:token", s.serveClientScript)
	}

	s.setupStaticFiles()
}

func (s *Server) setupStaticFiles() {
	subFS, err := fs.Sub(staticFS, "dist")
	if err != nil {
		return
	}

	s.router.GET("/assets/*filepath", func(c *gin.Context) {
		fp := c.Param("filepath")
		path := "assets" + fp
		data, err := fs.ReadFile(subFS, path)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}

		contentType := "application/octet-stream"
		switch {
		case strings.HasSuffix(fp, ".js"):
			contentType = "application/javascript; charset=utf-8"
		case strings.HasSuffix(fp, ".css"):
			contentType = "text/css; charset=utf-8"
		case strings.HasSuffix(fp, ".svg"):
			contentType = "image/svg+xml"
		case strings.HasSuffix(fp, ".png"):
			contentType = "image/png"
		case strings.HasSuffix(fp, ".jpg"), strings.HasSuffix(fp, ".jpeg"):
			contentType = "image/jpeg"
		case strings.HasSuffix(fp, ".woff2"):
			contentType = "font/woff2"
		case strings.HasSuffix(fp, ".woff"):
			contentType = "font/woff"
		}

		c.Data(http.StatusOK, contentType, data)
	})

	// vite.svg
	s.router.GET("/vite.svg", func(c *gin.Context) {
		data, _ := fs.ReadFile(subFS, "vite.svg")
		c.Data(http.StatusOK, "image/svg+xml", data)
	})

	// 婵犵數濮烽。钘壩ｉ崨鏉戠；闁规崘娉涚欢銈呂旈敐鍛殲闁稿顑夐弻锝呂熷▎鎯ф閺?
	s.router.GET("/", func(c *gin.Context) {
		data, _ := fs.ReadFile(subFS, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})

	s.router.NoRoute(func(c *gin.Context) {
		data, _ := fs.ReadFile(subFS, "index.html")
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	})
}

func (s *Server) Run() error {
	return s.router.Run(s.cfg.ListenAddr)
}

// RunWithContext starts the server and shuts down gracefully when ctx is cancelled.
func (s *Server) RunWithContext(ctx context.Context) error {
	srv := &http.Server{
		Addr:    s.cfg.ListenAddr,
		Handler: s.router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// ==================== 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犲綊鏌嶉崫鍕櫣闁活厽顨婇弻娑㈠箛閳轰礁顥嬪┑鐐村灟閸ㄥ湱绮绘导鏉戠閺夊牆澧界粔鍨箾?====================

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.GetHeader("Authorization")
		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}

		if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
		}

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}
		if temp2FA, ok := claims["temp_2fa"].(bool); ok && temp2FA {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "2fa verification required"})
			c.Abort()
			return
		}
		jti, ok := claims["jti"].(string)
		if !ok || jti == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session token"})
			c.Abort()
			return
		}

		if !s.svc.ValidateSession(jti) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired or invalid"})
			c.Abort()
			return
		}
		go s.svc.UpdateSessionActivity(jti)
		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])
		c.Set("jti", jti)
		c.Next()
	}
}

func (s *Server) wsAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := c.Query("token")
		if tokenStr == "" {
			tokenStr = c.GetHeader("Authorization")
			if len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
				tokenStr = tokenStr[7:]
			}
		}

		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			c.Abort()
			return
		}

		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}
		if temp2FA, ok := claims["temp_2fa"].(bool); ok && temp2FA {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "2fa verification required"})
			c.Abort()
			return
		}
		jti, ok := claims["jti"].(string)
		if !ok || jti == "" || !s.svc.ValidateSession(jti) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired or invalid"})
			c.Abort()
			return
		}
		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])
		c.Set("jti", jti)
		c.Next()
	}
}

func (s *Server) viewerWriteBlockMiddleware() gin.HandlerFunc {
	personalPaths := map[string]bool{
		"/api/change-password":     true,
		"/api/profile":             true,
		"/api/profile/2fa/enable":  true,
		"/api/profile/2fa/verify":  true,
		"/api/profile/2fa/disable": true,
		"/api/sessions/:id":        true,
		"/api/sessions/others":     true,
	}

	return func(c *gin.Context) {
		if c.Request.Method == "GET" {
			c.Next()
			return
		}

		if personalPaths[c.FullPath()] {
			c.Next()
			return
		}

		role, exists := c.Get("role")
		if exists {
			if r, ok := role.(string); ok && r == "viewer" {
				c.JSON(http.StatusForbidden, gin.H{"error": "只读用户无权执行此操作"})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}


type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := s.svc.ValidateUser(req.Username, req.Password)
	if err != nil {
		s.svc.LogOperation(0, req.Username, "login", "user", 0, "login failed", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		RecordLoginAttempt(false)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if !user.Enabled {
		s.svc.LogOperation(user.ID, user.Username, "login", "user", user.ID, "account disabled", c.ClientIP(), c.GetHeader("User-Agent"), "failed")
		c.JSON(http.StatusForbidden, gin.H{"error": "account is disabled"})
		return
	}

	if s.svc.IsEmailVerificationRequired() && !user.EmailVerified && user.Email != nil && *user.Email != "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "email not verified", "code": "EMAIL_NOT_VERIFIED"})
		return
	}

	if user.TwoFactorEnabled {
		tempToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id":  user.ID,
			"username": user.Username,
			"temp_2fa": true,
			"exp":      time.Now().Add(5 * time.Minute).Unix(),
		})

		tempTokenString, err := tempToken.SignedString([]byte(s.cfg.JWTSecret))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate temp token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"requires_2fa": true,
			"temp_token":   tempTokenString,
		})
		return
	}

	s.loginLimiter.Reset(c.ClientIP())
	RecordLoginAttempt(true)

	s.svc.UpdateUserLoginInfo(user.ID, c.ClientIP())

	s.svc.LogOperation(user.ID, user.Username, "login", "user", user.ID, "login success", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	jti := uuid.New().String()
	expiresAt := time.Now().Add(24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"jti":      jti,
		"exp":      expiresAt.Unix(),
	})

	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	if err := s.svc.CreateUserSession(user.ID, jti, c.ClientIP(), c.GetHeader("User-Agent"), expiresAt); err != nil {
		s.svc.LogOperation(user.ID, user.Username, "session_create", "user_session", 0, fmt.Sprintf("failed to create session: %v", err), c.ClientIP(), c.GetHeader("User-Agent"), "failed")
	}

	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
		"user": gin.H{
			"id":                user.ID,
			"username":          user.Username,
			"email":             user.Email,
			"role":              user.Role,
			"email_verified":    user.EmailVerified,
			"password_changed":  user.PasswordChanged,
			"plan":              user.Plan,
			"plan_id":           user.PlanID,
			"plan_start_at":     user.PlanStartAt,
			"plan_expire_at":    user.PlanExpireAt,
			"plan_traffic_used": user.PlanTrafficUsed,
		},
	})
}


type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

func (s *Server) register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := s.svc.RegisterUser(req.Username, req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !user.EmailVerified && user.VerificationToken != "" {
		go s.sendVerificationEmail(user)
	}

	s.svc.LogOperation(user.ID, user.Username, "register", "user", user.ID, "user registered", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{
		"message":            "Registration successful",
		"email_verification": !user.EmailVerified,
	})
}

func (s *Server) verifyEmail(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := s.svc.VerifyEmail(req.Token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	go s.sendWelcomeEmail(user)

	c.JSON(http.StatusOK, gin.H{"message": "Email verified successfully"})
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (s *Server) forgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, token, err := s.svc.RequestPasswordReset(req.Email)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link has been sent"})
		return
	}

	go s.sendPasswordResetEmail(user, token)

	c.JSON(http.StatusOK, gin.H{"message": "If the email exists, a reset link has been sent"})
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func (s *Server) resetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.ResetPassword(req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
}

func (s *Server) getRegistrationStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled":            s.svc.IsRegistrationEnabled(),
		"email_verification": s.svc.IsEmailVerificationRequired(),
	})
}

func (s *Server) adminVerifyUserEmail(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.UpdateUser(uint(id), map[string]interface{}{
		"email_verified":     true,
		"verification_token": "",
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) resendVerificationEmail(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	token, err := s.svc.ResendVerificationEmail(uint(id))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, _ := s.svc.GetUser(uint(id))
	if user != nil {
		user.VerificationToken = token
		go s.sendVerificationEmail(user)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) resetUserQuota(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.ResetUserQuota(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) sendVerificationEmail(user *model.User) {
	if user.Email == nil || *user.Email == "" {
		return
	}
	emailSender := s.getEmailSender()
	if emailSender == nil {
		return
	}
	emailSender.SendVerificationEmail(*user.Email, user.Username, user.VerificationToken)
}

func (s *Server) sendPasswordResetEmail(user *model.User, token string) {
	if user.Email == nil || *user.Email == "" {
		return
	}
	emailSender := s.getEmailSender()
	if emailSender == nil {
		return
	}
	emailSender.SendPasswordResetEmail(*user.Email, user.Username, token)
}

func (s *Server) sendWelcomeEmail(user *model.User) {
	if user.Email == nil || *user.Email == "" {
		return
	}
	emailSender := s.getEmailSender()
	if emailSender == nil {
		return
	}
	emailSender.SendWelcomeEmail(*user.Email, user.Username)
}

func (s *Server) getEmailSender() *notify.EmailSender {
	channels, err := s.svc.ListNotifyChannels()
	if err != nil {
		return nil
	}

	for _, ch := range channels {
		if (ch.Type == "smtp" || ch.Type == "email") && ch.Enabled {
			var smtpConfig model.SMTPConfig
			if err := json.Unmarshal([]byte(ch.Config), &smtpConfig); err != nil {
				continue
			}
			siteName := s.svc.GetSiteConfig(model.ConfigSiteName)
			siteURL := s.svc.GetSiteConfig(model.ConfigSiteURL)
			if siteName == "" {
				siteName = "GOST Panel"
			}
			return notify.NewEmailSender(&smtpConfig, siteName, siteURL)
		}
	}
	return nil
}

func (s *Server) getStats(c *gin.Context) {
	stats, err := s.svc.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (s *Server) healthCheck(c *gin.Context) {
	dbOk := true
	dbStatusStr := "ok"
	if err := s.svc.Ping(); err != nil {
		dbStatusStr = "error"
		dbOk = false
	}
	UpdateDBStatus(dbOk)

	stats, _ := s.svc.GetStats()
	nodeCount := 0
	onlineNodes := 0
	clientCount := 0
	onlineClients := 0
	userCount := 0
	if stats != nil {
		nodeCount = stats.TotalNodes
		onlineNodes = stats.OnlineNodes
		clientCount = stats.TotalClients
		onlineClients = stats.OnlineClients
		userCount = stats.TotalUsers
	}

	UpdateNodeMetrics(nodeCount, onlineNodes)
	UpdateClientMetrics(clientCount, onlineClients)
	UpdateUserMetrics(userCount)

	c.JSON(http.StatusOK, gin.H{
		"status":         "ok",
		"database":       dbStatusStr,
		"version":        CurrentAgentVersion,
		"nodes":          nodeCount,
		"online_nodes":   onlineNodes,
		"clients":        clientCount,
		"online_clients": onlineClients,
		"users":          userCount,
	})
}

var allowedScripts = map[string]bool{
	"install-node.sh":    true,
	"install-client.sh":  true,
	"install-node.ps1":   true,
	"install-client.ps1": true,
}

// serveInstallScript returns a handler that serves install scripts
func (s *Server) serveInstallScript(scriptName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !allowedScripts[scriptName] {
			c.JSON(http.StatusForbidden, gin.H{"error": "script not allowed"})
			return
		}

		// Try multiple paths
		paths := []string{
			filepath.Join("scripts", scriptName),
			filepath.Join(".", "scripts", scriptName),
		}

		var scriptPath string
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				scriptPath = p
				break
			}
		}

		if scriptPath == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "script not found"})
			return
		}

		content, err := os.ReadFile(scriptPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Set content type based on extension
		if filepath.Ext(scriptName) == ".ps1" {
			c.Header("Content-Type", "text/plain; charset=utf-8")
		} else {
			c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
		}
		c.Header("Content-Disposition", "inline; filename="+scriptName)

		c.String(http.StatusOK, string(content))
	}
}

func (s *Server) serveClientScript(c *gin.Context) {
	token := c.Param("token")

	client, err := s.svc.GetClientByToken(token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	panelURL := s.getPanelURL(c)

	script := fmt.Sprintf(`#!/bin/bash
# GOST 闂傚倸鍊峰ù鍥敋瑜庨〃銉╁传閵壯傜瑝閻庡箍鍎遍ˇ顖炲垂閸屾稓绠剧€瑰壊鍠曠花濠氭煛閸曗晛鍔滅紒缁樼洴楠炲鎮欑€靛憡顓婚梻浣告啞椤ㄥ棛鍠婂澶娢﹂柛鏇ㄥ灠閸愨偓闂侀潧顭俊鍥р枔閵堝鈷戦柛婵嗗椤ョ偞淇婇銏犳殻妤犵偛鍟撮弻銊р偓锝庡亜椤庢捇姊洪崨濠勨槈闁挎洏鍊曢埢鎾崇暆閸曨兘鎷洪梺鍛婄☉閿曘儳浜搁銏＄厪闁割偁鍩勯悞鐐亜?(Agent 濠电姷鏁告慨鐑姐€傞挊澹╋綁宕ㄩ弶鎴濈€銈呯箰閻楀棝鎮為崹顐犱簻闁瑰搫妫楁禍鍓х磼閸撗嗘闁告ɑ鍎抽埥澶愭偨缁嬭法鍔?
# 闂傚倸鍊峰ù鍥敋瑜庨〃銉╁传閵壯傜瑝閻庡箍鍎遍ˇ顖炲垂閸屾稓绠剧€瑰壊鍠曠花濠氭煛閸曗晛鍔滅紒缁樼洴楠炲鎮欑€靛憡顓婚梻? %s

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

REPO="supernaga/gostpanel"
PANEL_URL="%s"
CLIENT_TOKEN="%s"
INSTALL_DIR="/opt/gost-panel"

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

dl() {
    local url="$1" output="$2"
    if command -v curl &>/dev/null; then
        [ -n "$output" ] && curl -fsSL "$url" -o "$output" || curl -fsSL "$url"
    elif command -v wget &>/dev/null; then
        [ -n "$output" ] && wget -qO "$output" "$url" || wget -qO- "$url"
    else
        log_error "curl and wget not found"; exit 1
    fi
}

detect_arch() {
    local arch=$(uname -m)
    case $arch in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        armv7l|armv7) echo "armv7" ;;
        armv6l|armv6) echo "armv6" ;;
        armv5*) echo "armv5" ;;
        mips) echo "mipsle" ;;
        mips64) echo "mips64le" ;;
        i386|i686) echo "386" ;;
        *) log_error "Unsupported architecture: $arch"; exit 1 ;;
    esac
}

echo "======================================"
echo "  GOST Panel Client Installer"
echo "  (Agent Mode - Built-in Heartbeat)"
echo "======================================"
echo ""

GOST_ARCH=$(detect_arch)
log_info "Architecture: $GOST_ARCH"
log_info "Panel: $PANEL_URL"

# 濠电姷鏁告慨鐑藉极閹间礁纾婚柣鎰惈缁犱即鏌熼梻瀵割槮缂佺姷濞€閺岀喖鎮ч崼鐔哄嚒缂備胶濮甸悧鏇㈠煘閹达附鍋愰柛娆忣槹閹瑧绱撴笟鍥т簻缂佸鐖奸獮澶婎潰瀹€鈧悿鈧梺鍦檸閸ㄩ亶鎮?shell 闂傚倸鍊搁崐鎼佲€﹂鍕；闁告洦鍊嬪ú顏勎у璺猴功閺屽牓姊虹憴鍕姸濠殿喓鍊濆畷?(婵犵數濮烽弫鎼佸磻濞戙埄鏁嬫い鎾跺枑閸欏繘鎮楅悽鐢点€婇柛瀣尭閳藉骞掗幘瀵稿絽闂佹崘宕甸崑鐐哄Φ閸曨垰绠涢柛顐ｆ礃椤庡秹姊虹粙娆惧剰妞ゆ垵顦靛璇测槈閵忊晜鏅濋梺闈涚墕閹冲繘鎮楁ィ鍐┾拺闁革富鍘奸崢鏉懨瑰鍐煟鐎殿喛顕ч埥澶娢熷鍕棃闁糕斁鍋撳銈嗗坊閸嬫捇鎮￠妶澶嬬厪濠电姴绻愰惁婊勭箾?
if [ -f /etc/gost/heartbeat.sh ]; then
    log_info "Cleaning up old heartbeat..."
    systemctl stop gost-heartbeat.timer 2>/dev/null || true
    systemctl disable gost-heartbeat.timer 2>/dev/null || true
    rm -f /etc/systemd/system/gost-heartbeat.service
    rm -f /etc/systemd/system/gost-heartbeat.timer
    (crontab -l 2>/dev/null | grep -v "gost/heartbeat") | crontab - 2>/dev/null || true
    rm -f /etc/gost/heartbeat.sh
    systemctl stop gost 2>/dev/null || true
    systemctl disable gost 2>/dev/null || true
    rm -f /etc/systemd/system/gost.service
    systemctl daemon-reload 2>/dev/null || true
fi
if systemctl is-active gost-client &>/dev/null 2>&1; then
    systemctl stop gost-client 2>/dev/null || true
    systemctl disable gost-client 2>/dev/null || true
    rm -f /etc/systemd/system/gost-client.service
    systemctl daemon-reload 2>/dev/null || true
fi

# 婵犵數濮烽弫鎼佸磻閻愬搫鍨傞柛顐ｆ礀缁犱即鏌熼梻瀵歌窗闁轰礁瀚伴弻娑㈩敃閿濆洩绌?Agent
log_info "[1/3] Installing Agent..."
mkdir -p "$INSTALL_DIR"
rm -f "$INSTALL_DIR/gost-agent"

latest_version=$(dl "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
[ -z "$latest_version" ] && latest_version="v1.0.0"

agent_url="https://github.com/$REPO/releases/download/$latest_version/gost-agent-linux-$GOST_ARCH"
log_info "Downloading agent ($latest_version)..."

if dl "$agent_url" "$INSTALL_DIR/gost-agent" 2>/dev/null; then
    chmod +x "$INSTALL_DIR/gost-agent"
    log_info "Agent downloaded"
else
    log_warn "GitHub download failed, trying panel..."
    if dl "$PANEL_URL/agent/download/linux/$GOST_ARCH" "$INSTALL_DIR/gost-agent" 2>/dev/null; then
        chmod +x "$INSTALL_DIR/gost-agent"
    else
        log_error "Failed to download agent"; exit 1
    fi
fi

# 闂傚倸鍊峰ù鍥敋瑜嶉湁闁绘垼妫勭粻鐘绘煙閹冩闁搞儺鍓欑粻顕€鏌涢幘宕囦虎妞わ附澹嗛幑銏犫攽鐎ｎ亞鍊為悷婊冪Ч閹潧顫滈埀顒勫箖濡ゅ懐宓侀柛顭戝枛婵酣姊洪悷鏉挎毐闂佸府缍侀弫?
log_info "[2/3] Installing service..."
$INSTALL_DIR/gost-agent service install -panel $PANEL_URL -token $CLIENT_TOKEN -mode client
$INSTALL_DIR/gost-agent service start

# 闂傚倸鍊峰ù鍥敋瑜嶉湁闁绘垼妫勭粻鐘绘煙閹规劦鍤欓悗姘槹閵囧嫰骞掗幋婵愪患闂?
log_info "[3/3] Done!"
echo ""
echo "======================================"
echo "  Installation Complete!"
echo "======================================"
echo ""
echo "Agent Mode Features:"
echo "  - Built-in heartbeat (every 30s)"
echo "  - Auto config reload"
echo "  - Auto GOST download"
echo "  - Auto uninstall when deleted from panel"
echo ""
echo "Commands:"
echo "  $INSTALL_DIR/gost-agent service status   - Check status"
echo "  $INSTALL_DIR/gost-agent service restart  - Restart"
echo "  journalctl -u gost-client -f             - View logs"
`, client.Name, panelURL, client.Token)

	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.String(http.StatusOK, script)
}
