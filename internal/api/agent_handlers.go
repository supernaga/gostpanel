// Agent 接口模块
package api

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"

	"github.com/supernaga/gost-panel/internal/gost"
	"github.com/gin-gonic/gin"
)



// ==================== Agent 接口 ====================

type AgentRegisterRequest struct {
	Token   string `json:"token" binding:"required"`
	Type    string `json:"type"` // node/client
	Version string `json:"version"`
}

func (s *Server) agentRegister(c *gin.Context) {
	var req AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 尝试查找节点
	node, err := s.svc.GetNodeByToken(req.Token)
	if err == nil {
		s.svc.UpdateNodeStatus(node.ID, "online", 0, 0, 0)
		c.JSON(http.StatusOK, gin.H{
			"type":    "node",
			"id":      node.ID,
			"message": "registered",
		})
		return
	}

	// 尝试查找客户端
	client, err := s.svc.GetClientByToken(req.Token)
	if err == nil {
		s.svc.UpdateClient(client.ID, map[string]interface{}{"status": "online", "last_seen": time.Now()})
		c.JSON(http.StatusOK, gin.H{
			"type":    "client",
			"id":      client.ID,
			"message": "registered",
		})
		return
	}

	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
}

type AgentHeartbeatRequest struct {
	Token        string                      `json:"token" binding:"required"`
	Connections  int                         `json:"connections"`
	TrafficIn    int64                       `json:"traffic_in"`
	TrafficOut   int64                       `json:"traffic_out"`
	ConfigHash   string                      `json:"config_hash"`   // 当前配置的哈希值
	AgentVersion string                      `json:"agent_version"` // Agent 版本
	ServiceStats map[string]map[string]int64 `json:"service_stats"` // 按服务名分类的统计
}

func (s *Server) agentHeartbeat(c *gin.Context) {
	var req AgentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 尝试更新节点
	node, err := s.svc.GetNodeByToken(req.Token)
	if err == nil {
		s.svc.UpdateNodeStatus(node.ID, "online", req.Connections, req.TrafficIn, req.TrafficOut)
		// 广播节点状态更新
		s.BroadcastNodeStatus(node.ID, "online", req.Connections, node.TrafficIn+req.TrafficIn, node.TrafficOut+req.TrafficOut)

		// 处理服务级别统计 (隧道流量)
		if req.ServiceStats != nil {
			s.processServiceStats(node.ID, req.ServiceStats)
		}

		// 检查配置是否需要更新
		reloadConfig := false
		if req.ConfigHash != "" {
			// 计算当前节点配置的哈希值
			currentHash := s.svc.GetNodeConfigHash(node.ID)
			if currentHash != req.ConfigHash {
				reloadConfig = true
			}
		}

		// 检查 Agent 是否需要更新
		needsUpdate, forceUpdate := s.checkAgentNeedsUpdate(req.AgentVersion)

		c.JSON(http.StatusOK, gin.H{
			"status":        "ok",
			"reload_config": reloadConfig,
			"needs_update":  needsUpdate,
			"force_update":  forceUpdate,
		})
		return
	}

	// 尝试更新客户端
	client, err := s.svc.GetClientByToken(req.Token)
	if err == nil {
		s.svc.UpdateClient(client.ID, map[string]interface{}{
			"status":      "online",
			"last_seen":   time.Now(),
			"traffic_in":  client.TrafficIn + req.TrafficIn,
			"traffic_out": client.TrafficOut + req.TrafficOut,
		})

		// 检查配置是否需要更新（包括关联节点的密码变更）
		reloadConfig := false
		if req.ConfigHash != "" {
			currentHash := s.svc.GetClientConfigHash(client.ID)
			if currentHash != req.ConfigHash {
				reloadConfig = true
			}
		}

		// 检查 Agent 是否需要更新
		needsUpdate, forceUpdate := s.checkAgentNeedsUpdate(req.AgentVersion)

		c.JSON(http.StatusOK, gin.H{
			"status":        "ok",
			"reload_config": reloadConfig,
			"needs_update":  needsUpdate,
			"force_update":  forceUpdate,
		})
		return
	}

	// Token 无效，通知 Agent 卸载自己（附带签名验证）
	uninstallSig := fmt.Sprintf("%x", sha256.Sum256([]byte("uninstall:"+req.Token)))
	c.JSON(http.StatusUnauthorized, gin.H{
		"error":         "invalid token",
		"uninstall":     true,
		"uninstall_sig": uninstallSig,
	})
}

// checkAgentNeedsUpdate 检查 Agent 是否需要更新
func (s *Server) checkAgentNeedsUpdate(clientVersion string) (needsUpdate, forceUpdate bool) {
	if clientVersion == "" {
		return false, false
	}

	// 检查是否开启自动更新
	autoUpdate := s.svc.GetSiteConfig("agent_auto_update")
	if autoUpdate != "true" {
		return false, false
	}

	// 比较版本
	needsUpdate = compareVersions(clientVersion, CurrentAgentVersion) < 0

	// 检查是否强制更新
	if needsUpdate {
		forceUpdateConfig := s.svc.GetSiteConfig("agent_force_update")
		forceUpdate = forceUpdateConfig == "true"
	}

	return needsUpdate, forceUpdate
}

// processServiceStats 处理按服务分类的流量统计
func (s *Server) processServiceStats(nodeID uint, stats map[string]map[string]int64) {
	for serviceName, serviceStats := range stats {
		trafficIn := serviceStats["traffic_in"]
		trafficOut := serviceStats["traffic_out"]

		// 解析服务名，匹配隧道或客户端
		// 隧道服务名格式: tunnel-{id}-tcp, tunnel-{id}-udp, tunnel-{id}
		// 客户端服务名格式: rtcp-tunnel, rudp-tunnel, client-{id}
		if tunnelID := parseTunnelID(serviceName); tunnelID > 0 {
			s.svc.UpdateTunnelTraffic(uint(tunnelID), trafficIn, trafficOut)
		} else if clientID := parseClientID(serviceName); clientID > 0 {
			s.svc.UpdateClientTraffic(uint(clientID), trafficIn, trafficOut)
		}
	}
}

// parseTunnelID 从服务名解析隧道ID
func parseTunnelID(serviceName string) int {
	// 匹配 tunnel-{id}, tunnel-{id}-tcp, tunnel-{id}-udp
	var id int
	if n, _ := fmt.Sscanf(serviceName, "tunnel-%d-tcp", &id); n == 1 {
		return id
	}
	if n, _ := fmt.Sscanf(serviceName, "tunnel-%d-udp", &id); n == 1 {
		return id
	}
	if n, _ := fmt.Sscanf(serviceName, "tunnel-%d", &id); n == 1 {
		return id
	}
	return 0
}

// parseClientID 从服务名解析客户端ID
func parseClientID(serviceName string) int {
	var id int
	if n, _ := fmt.Sscanf(serviceName, "client-%d", &id); n == 1 {
		return id
	}
	return 0
}

func (s *Server) agentGetConfig(c *gin.Context) {
	token := c.Param("token")

	// 尝试查找节点
	node, err := s.svc.GetNodeByToken(token)
	if err == nil {
		// 使用 ConfigGenerator 生成完整配置（包含规则）
		generator := gost.NewConfigGenerator()
		bypasses, _ := s.svc.GetBypassesByNode(node.ID)
		admissions, _ := s.svc.GetAdmissionsByNode(node.ID)
		hostMappings, _ := s.svc.GetHostMappingsByNode(node.ID)
		ingresses, _ := s.svc.GetIngressesByNode(node.ID)
		config := generator.GenerateNodeConfigWithRules(node, bypasses, admissions, hostMappings, ingresses)
		c.YAML(http.StatusOK, config)
		return
	}

	// 尝试查找客户端
	client, err := s.svc.GetClientByToken(token)
	if err == nil {
		config := s.generateClientConfig(client)
		c.YAML(http.StatusOK, config)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
}

// ==================== GOST 操作 ====================

