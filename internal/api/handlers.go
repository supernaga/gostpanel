// TODO: This file is too large (5000+ lines). Consider splitting into separate files:
// node_handlers.go, client_handlers.go, user_handlers.go, tunnel_handlers.go, etc.
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AliceNetworks/gost-panel/internal/gost"
	"github.com/AliceNetworks/gost-panel/internal/model"
	"github.com/AliceNetworks/gost-panel/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
)

// ==================== 辅助函数 ====================

// getUserInfo 从 JWT context 获取用户信息
func getUserInfo(c *gin.Context) (userID uint, isAdmin bool) {
	userIDFloat, _ := c.Get("user_id")
	role, _ := c.Get("role")

	if userIDFloat != nil {
		if id, ok := userIDFloat.(float64); ok {
			userID = uint(id)
		}
	}
	if role != nil {
		if r, ok := role.(string); ok {
			isAdmin = r == "admin"
		}
	}
	return
}

// parseID 从 URL 参数中解析 ID，失败时返回 400 错误
func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID 参数"})
		return 0, false
	}
	return uint(id), true
}

// getPanelURL 获取 Panel 的完整 URL（支持反向代理）
func (s *Server) getPanelURL(c *gin.Context) string {
	// 优先使用系统设置中的站点 URL
	if siteURL := s.svc.GetSiteConfig(model.ConfigSiteURL); siteURL != "" {
		return strings.TrimSuffix(siteURL, "/")
	}

	// 回退: 从请求头自动检测
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}

	scheme := "http"
	if c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	} else if strings.EqualFold(c.GetHeader("X-Forwarded-Ssl"), "on") {
		scheme = "https"
	} else if c.GetHeader("X-Forwarded-Port") == "443" {
		scheme = "https"
	} else if c.Request.TLS != nil {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

// ==================== 节点管理 ====================

func (s *Server) listNodes(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	nodes, err := s.svc.ListNodesByOwner(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

func (s *Server) listNodesPaginated(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")
	sortBy := c.Query("sort_by")
	sortDesc := c.Query("sort_desc") == "true"

	params := service.NewPaginationParams(page, pageSize, search)
	params.SortBy = sortBy
	params.SortDesc = sortDesc

	result, err := s.svc.ListNodesPaginated(userID, isAdmin, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) getNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}
	c.JSON(http.StatusOK, node)
}

type CreateNodeRequest struct {
	Name          string `json:"name" binding:"required"`
	Host          string `json:"host" binding:"required"`
	Port          int    `json:"port"`
	APIPort       int    `json:"api_port"`
	APIUser       string `json:"api_user"`
	APIPass       string `json:"api_pass"`
	ProxyUser     string `json:"proxy_user"`
	ProxyPass     string `json:"proxy_pass"`
	TrafficQuota  int64  `json:"traffic_quota"`
	QuotaResetDay int    `json:"quota_reset_day"`
	// 协议配置
	Protocol      string `json:"protocol"`       // socks5/http/ss/socks4/http2/ssu/auto/relay
	Transport     string `json:"transport"`      // tcp/tls/ws/wss/h2/h2c/quic/kcp
	TransportOpts string `json:"transport_opts"` // 传输层配置 JSON
	// Shadowsocks
	SSMethod   string `json:"ss_method"`
	SSPassword string `json:"ss_password"`
	// TLS
	TLSEnabled  bool   `json:"tls_enabled"`
	TLSCertFile string `json:"tls_cert_file"`
	TLSKeyFile  string `json:"tls_key_file"`
	TLSSNI      string `json:"tls_sni"`
	// WebSocket
	WSPath string `json:"ws_path"`
	WSHost string `json:"ws_host"`
	// 限速
	SpeedLimit    int64 `json:"speed_limit"`
	ConnRateLimit int   `json:"conn_rate_limit"`
	// DNS
	DNSServer string `json:"dns_server"`
}

func (s *Server) createNode(c *gin.Context) {
	var req CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "node")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}
	}

	node := &model.Node{
		Name:          req.Name,
		Host:          req.Host,
		Port:          req.Port,
		APIPort:       req.APIPort,
		APIUser:       req.APIUser,
		APIPass:       req.APIPass,
		ProxyUser:     req.ProxyUser,
		ProxyPass:     req.ProxyPass,
		TrafficQuota:  req.TrafficQuota,
		QuotaResetDay: req.QuotaResetDay,
		Protocol:      req.Protocol,
		Transport:     req.Transport,
		TransportOpts: req.TransportOpts,
		SSMethod:      req.SSMethod,
		SSPassword:    req.SSPassword,
		TLSEnabled:    req.TLSEnabled,
		TLSCertFile:   req.TLSCertFile,
		TLSKeyFile:    req.TLSKeyFile,
		TLSSNI:        req.TLSSNI,
		WSPath:        req.WSPath,
		WSHost:        req.WSHost,
		SpeedLimit:    req.SpeedLimit,
		ConnRateLimit: req.ConnRateLimit,
		DNSServer:     req.DNSServer,
		OwnerID:       &userID,
	}

	// 默认值
	if node.Port == 0 {
		node.Port = 38567
	}
	if node.APIPort == 0 {
		node.APIPort = 18080
	}
	if node.QuotaResetDay == 0 {
		node.QuotaResetDay = 1
	}
	if node.Protocol == "" {
		node.Protocol = "socks5"
	}
	if node.Transport == "" {
		node.Transport = "tcp"
	}

	if err := s.svc.CreateNode(node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, node)
}

func (s *Server) updateNode(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetNodeByOwner(id, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 不允许更新的字段
	delete(updates, "id")
	delete(updates, "agent_token")
	delete(updates, "created_at")
	delete(updates, "owner_id")

	// 密码字段为空时不更新，防止编辑时误覆盖已有密码
	for _, key := range []string{"api_pass", "proxy_pass", "ss_password"} {
		if v, ok := updates[key]; ok && v == "" {
			delete(updates, key)
		}
	}

	if err := s.svc.UpdateNode(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteNode(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetNodeByOwner(id, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.svc.DeleteNode(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 批量操作 ====================

// BatchOperationRequest 批量操作请求
type BatchOperationRequest struct {
	IDs []uint `json:"ids" binding:"required"`
}

// batchDeleteNodes 批量删除节点
func (s *Server) batchDeleteNodes(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no nodes selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("nodes", req.IDs, userID, isAdmin)

	successCount := 0
	failCount := 0
	for _, id := range allowedIDs {
		if err := s.svc.DeleteNode(id); err != nil {
			failCount++
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": successCount,
		"failed":  failCount,
		"message": fmt.Sprintf("成功删除 %d 个节点，失败 %d 个", successCount, failCount),
	})
}

// batchSyncNodes 批量同步节点配置
func (s *Server) batchSyncNodes(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no nodes selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("nodes", req.IDs, userID, isAdmin)

	successCount := 0
	failCount := 0
	offlineCount := 0
	for _, id := range allowedIDs {
		node, err := s.svc.GetNode(id)
		if err != nil {
			failCount++
			continue
		}
		if node.Status != "online" {
			offlineCount++
			continue
		}
		// 标记节点需要重新加载配置
		s.svc.TouchNode(id)
		successCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": successCount,
		"offline": offlineCount,
		"failed":  failCount,
		"message": fmt.Sprintf("成功同步 %d 个节点，%d 个离线，%d 个失败", successCount, offlineCount, failCount),
	})
}

// batchEnableNodes 批量启用节点
func (s *Server) batchEnableNodes(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no nodes selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("nodes", req.IDs, userID, isAdmin)

	result := s.svc.DB().Model(&model.Node{}).Where("id IN ?", allowedIDs).Update("status", "online")

	c.JSON(http.StatusOK, gin.H{
		"success": result.RowsAffected,
		"message": fmt.Sprintf("成功启用 %d 个节点", result.RowsAffected),
	})
}

// batchDisableNodes 批量禁用节点
func (s *Server) batchDisableNodes(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no nodes selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("nodes", req.IDs, userID, isAdmin)

	result := s.svc.DB().Model(&model.Node{}).Where("id IN ?", allowedIDs).Update("status", "offline")

	c.JSON(http.StatusOK, gin.H{
		"success": result.RowsAffected,
		"message": fmt.Sprintf("成功禁用 %d 个节点", result.RowsAffected),
	})
}

// batchDeleteClients 批量删除客户端
func (s *Server) batchDeleteClients(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no clients selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("clients", req.IDs, userID, isAdmin)

	successCount := 0
	failCount := 0
	for _, id := range allowedIDs {
		if err := s.svc.DeleteClient(id); err != nil {
			failCount++
		} else {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": successCount,
		"failed":  failCount,
		"message": fmt.Sprintf("成功删除 %d 个客户端，失败 %d 个", successCount, failCount),
	})
}

// batchEnableClients 批量启用客户端
func (s *Server) batchEnableClients(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no clients selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("clients", req.IDs, userID, isAdmin)

	result := s.svc.DB().Model(&model.Client{}).Where("id IN ?", allowedIDs).Update("status", "connected")

	c.JSON(http.StatusOK, gin.H{
		"success": result.RowsAffected,
		"message": fmt.Sprintf("成功启用 %d 个客户端", result.RowsAffected),
	})
}

// batchDisableClients 批量禁用客户端
func (s *Server) batchDisableClients(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no clients selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("clients", req.IDs, userID, isAdmin)

	result := s.svc.DB().Model(&model.Client{}).Where("id IN ?", allowedIDs).Update("status", "disconnected")

	c.JSON(http.StatusOK, gin.H{
		"success": result.RowsAffected,
		"message": fmt.Sprintf("成功禁用 %d 个客户端", result.RowsAffected),
	})
}

// batchSyncClients 批量同步客户端配置
func (s *Server) batchSyncClients(c *gin.Context) {
	var req BatchOperationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no clients selected"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	allowedIDs := s.svc.FilterIDsByOwner("clients", req.IDs, userID, isAdmin)

	successCount := 0
	failCount := 0
	offlineCount := 0
	for _, id := range allowedIDs {
		client, err := s.svc.GetClient(id)
		if err != nil {
			failCount++
			continue
		}
		if client.Status != "connected" {
			offlineCount++
			continue
		}
		// 标记客户端需要重新加载配置
		s.svc.DB().Model(&model.Client{}).Where("id = ?", id).Update("updated_at", time.Now())
		successCount++
	}

	c.JSON(http.StatusOK, gin.H{
		"success": successCount,
		"offline": offlineCount,
		"failed":  failCount,
		"message": fmt.Sprintf("成功同步 %d 个客户端，%d 个离线，%d 个失败", successCount, offlineCount, failCount),
	})
}

func (s *Server) applyNodeConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}

	if err := s.svc.ApplyNodeConfig(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) cloneNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	cloned := &model.Node{
		Name:             node.Name + " (副本)",
		Host:             node.Host,
		Port:             node.Port + 1,
		APIPort:          node.APIPort + 1,
		APIUser:          node.APIUser,
		APIPass:          node.APIPass,
		ProxyUser:        node.ProxyUser,
		ProxyPass:        node.ProxyPass,
		Protocol:         node.Protocol,
		Transport:        node.Transport,
		TransportOpts:    node.TransportOpts,
		SSMethod:         node.SSMethod,
		SSPassword:       node.SSPassword,
		TLSEnabled:       node.TLSEnabled,
		TLSCertFile:      node.TLSCertFile,
		TLSKeyFile:       node.TLSKeyFile,
		TLSSNI:           node.TLSSNI,
		TLSALPN:          node.TLSALPN,
		WSPath:           node.WSPath,
		WSHost:           node.WSHost,
		SpeedLimit:       node.SpeedLimit,
		ConnRateLimit:    node.ConnRateLimit,
		DNSServer:        node.DNSServer,
		ProxyProtocol:    node.ProxyProtocol,
		ProbeResist:      node.ProbeResist,
		ProbeResistValue: node.ProbeResistValue,
		PluginConfig:     node.PluginConfig,
		TrafficQuota:     node.TrafficQuota,
		QuotaResetDay:    node.QuotaResetDay,
		OwnerID:          &userID,
	}

	if err := s.svc.CreateNode(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "node", cloned.ID, fmt.Sprintf("from #%d", node.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneClient(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	cloned := &model.Client{
		Name:          client.Name + " (副本)",
		NodeID:        client.NodeID,
		LocalPort:     client.LocalPort + 1,
		RemotePort:    client.RemotePort + 1,
		ProxyUser:     client.ProxyUser,
		ProxyPass:     client.ProxyPass,
		TrafficQuota:  client.TrafficQuota,
		QuotaResetDay: client.QuotaResetDay,
		OwnerID:       &userID,
	}

	if err := s.svc.CreateClient(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "client", cloned.ID, fmt.Sprintf("from #%d", client.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) clonePortForward(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	forward, err := s.svc.GetPortForwardByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "port forward not found"})
		return
	}

	// 检查权限
	if !isAdmin && forward.OwnerID != nil && *forward.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// 解析地址以递增端口
	localAddr := forward.LocalAddr
	if host, port, err := net.SplitHostPort(forward.LocalAddr); err == nil {
		if p, err := strconv.Atoi(port); err == nil {
			localAddr = net.JoinHostPort(host, strconv.Itoa(p+1))
		}
	}

	cloned := &model.PortForward{
		Name:       forward.Name + " (副本)",
		NodeID:     forward.NodeID,
		Type:       forward.Type,
		LocalAddr:  localAddr,
		RemoteAddr: forward.RemoteAddr,
		ChainID:    forward.ChainID,
		Enabled:    forward.Enabled,
		OwnerID:    &userID,
	}

	if err := s.svc.CreatePortForward(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "port_forward", cloned.ID, fmt.Sprintf("from #%d", forward.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	// 检查权限
	if !isAdmin && tunnel.OwnerID != nil && *tunnel.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	cloned := &model.Tunnel{
		Name:          tunnel.Name + " (副本)",
		Description:   tunnel.Description,
		EntryNodeID:   tunnel.EntryNodeID,
		EntryPort:     tunnel.EntryPort + 1,
		Protocol:      tunnel.Protocol,
		ExitNodeID:    tunnel.ExitNodeID,
		TargetAddr:    tunnel.TargetAddr,
		Enabled:       tunnel.Enabled,
		TrafficQuota:  tunnel.TrafficQuota,
		QuotaResetDay: tunnel.QuotaResetDay,
		SpeedLimit:    tunnel.SpeedLimit,
		OwnerID:       &userID,
	}

	if err := s.svc.CreateTunnel(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "tunnel", cloned.ID, fmt.Sprintf("from #%d", tunnel.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneProxyChain(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	chain, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy chain not found"})
		return
	}

	// 检查权限
	if !isAdmin && chain.OwnerID != nil && *chain.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// 解析监听地址以递增端口
	listenAddr := chain.ListenAddr
	if host, port, err := net.SplitHostPort(chain.ListenAddr); err == nil {
		if p, err := strconv.Atoi(port); err == nil {
			listenAddr = net.JoinHostPort(host, strconv.Itoa(p+1))
		}
	}

	cloned := &model.ProxyChain{
		Name:        chain.Name + " (副本)",
		Description: chain.Description,
		ListenAddr:  listenAddr,
		ListenType:  chain.ListenType,
		TargetAddr:  chain.TargetAddr,
		Enabled:     chain.Enabled,
		OwnerID:     &userID,
	}

	if err := s.svc.CreateProxyChain(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 克隆跳点
	hops, err := s.svc.GetProxyChainHops(uint(id))
	if err == nil {
		for _, hop := range hops {
			clonedHop := &model.ProxyChainHop{
				ChainID:  cloned.ID,
				NodeID:   hop.NodeID,
				HopOrder: hop.HopOrder,
				Enabled:  hop.Enabled,
			}
			s.svc.AddProxyChainHop(clonedHop)
		}
	}

	s.audit.LogSuccess(c, "clone", "proxy_chain", cloned.ID, fmt.Sprintf("from #%d", chain.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	group, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node group not found"})
		return
	}

	// 检查权限
	if !isAdmin && group.OwnerID != nil && *group.OwnerID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	cloned := &model.NodeGroup{
		Name:          group.Name + " (副本)",
		Strategy:      group.Strategy,
		Selector:      group.Selector,
		FailTimeout:   group.FailTimeout,
		MaxFails:      group.MaxFails,
		HealthCheck:   group.HealthCheck,
		CheckInterval: group.CheckInterval,
		OwnerID:       &userID,
	}

	if err := s.svc.CreateNodeGroup(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 克隆成员
	members, err := s.svc.ListNodeGroupMembers(uint(id))
	if err == nil {
		for _, member := range members {
			clonedMember := &model.NodeGroupMember{
				GroupID:  cloned.ID,
				NodeID:   member.NodeID,
				Weight:   member.Weight,
				Priority: member.Priority,
				Enabled:  member.Enabled,
			}
			s.svc.AddNodeGroupMember(clonedMember)
		}
	}

	s.audit.LogSuccess(c, "clone", "node_group", cloned.ID, fmt.Sprintf("from #%d", group.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) syncNodeConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 生成配置并自动保存版本快照
	generator := gost.NewConfigGenerator()
	bypasses, _ := s.svc.GetBypassesByNode(node.ID)
	admissions, _ := s.svc.GetAdmissionsByNode(node.ID)
	hostMappings, _ := s.svc.GetHostMappingsByNode(node.ID)
	ingresses, _ := s.svc.GetIngressesByNode(node.ID)
	config := generator.GenerateNodeConfigWithRules(node, bypasses, admissions, hostMappings, ingresses)

	// 将配置序列化为 YAML 字符串并保存版本
	configYAML, err := yaml.Marshal(config)
	if err == nil {
		s.svc.SaveConfigVersion(uint(id), string(configYAML), "Auto-saved on sync")
		s.svc.CleanupOldVersions(uint(id), 20) // 保留最新 20 个版本
	}

	// 标记节点需要重新加载配置（通过更新 updated_at）
	s.svc.TouchNode(uint(id))

	// 根据节点状态返回不同提示
	msg := "配置已更新，Agent 将在下次心跳时自动同步（最多 30 秒）"
	if node.Status != "online" {
		msg = "配置已生成，Agent 上线后将自动加载最新配置"
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": msg,
	})
}

func (s *Server) getNodeGostConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 使用新的配置生成器
	generator := gost.NewConfigGenerator()
	bypasses, _ := s.svc.GetBypassesByNode(node.ID)
	admissions, _ := s.svc.GetAdmissionsByNode(node.ID)
	hostMappings, _ := s.svc.GetHostMappingsByNode(node.ID)
	ingresses, _ := s.svc.GetIngressesByNode(node.ID)
	config := generator.GenerateNodeConfigWithRules(node, bypasses, admissions, hostMappings, ingresses)

	c.YAML(http.StatusOK, config)
}

func (s *Server) getNodeInstallScript(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	osType := c.DefaultQuery("os", "linux")
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 获取正确的 Panel URL（支持反向代理）
	panelURL := s.getPanelURL(c)
	githubRaw := s.cfg.GitHubRawURL

	var script, oneLineCommand string

	if osType == "windows" {
		// PowerShell 安装命令
		oneLineCommand = fmt.Sprintf(`irm "%s/install-node.ps1" -OutFile "$env:TEMP\install-node.ps1"; & "$env:TEMP\install-node.ps1" -PanelUrl "%s" -Token "%s"`, githubRaw, panelURL, node.AgentToken)
		script = fmt.Sprintf(`# GOST Panel Node Installation for Windows
# Run in PowerShell as Administrator

# One-line install (copy and run this command):
%s
`, oneLineCommand)
	} else {
		// Bash 安装命令
		oneLineCommand = fmt.Sprintf(`(curl -fsSL "%s/install-node.sh" 2>/dev/null || wget -qO- "%s/install-node.sh") | bash -s -- -p "%s" -t "%s"`, githubRaw, githubRaw, panelURL, node.AgentToken)
		script = fmt.Sprintf(`#!/bin/bash
# GOST Panel Node Installation
# Supported: Linux (amd64, arm64, armv7, armv6, mips, mipsle)

# One-line install (copy and run this command):
%s

# Or with forced architecture (for cross-compilation):
# (curl -fsSL "%s/install-node.sh" 2>/dev/null || wget -qO- "%s/install-node.sh") | bash -s -- -p "%s" -t "%s" -a armv6
`, oneLineCommand, githubRaw, githubRaw, panelURL, node.AgentToken)
	}

	c.JSON(http.StatusOK, gin.H{
		"script":           script,
		"one_line_command": oneLineCommand,
	})
}

// ==================== 客户端管理 ====================

func (s *Server) listClients(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	clients, err := s.svc.ListClientsByOwner(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, clients)
}

func (s *Server) listClientsPaginated(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := c.Query("search")
	sortBy := c.Query("sort_by")
	sortDesc := c.Query("sort_desc") == "true"

	params := service.NewPaginationParams(page, pageSize, search)
	params.SortBy = sortBy
	params.SortDesc = sortDesc

	result, err := s.svc.ListClientsPaginated(userID, isAdmin, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) getClient(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}
	c.JSON(http.StatusOK, client)
}

type CreateClientRequest struct {
	Name          string `json:"name" binding:"required"`
	NodeID        uint   `json:"node_id" binding:"required"`
	LocalPort     int    `json:"local_port"`
	RemotePort    int    `json:"remote_port" binding:"required"`
	ProxyUser     string `json:"proxy_user"`
	ProxyPass     string `json:"proxy_pass"`
	TrafficQuota  int64  `json:"traffic_quota"`   // 流量配额 (bytes)
	QuotaResetDay int    `json:"quota_reset_day"` // 每月重置日
}

func (s *Server) createClient(c *gin.Context) {
	var req CreateClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "client")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		// 检查节点访问权限
		allowed, msg = s.svc.CheckPlanNodeAccess(userID, req.NodeID)
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		if _, err := s.svc.GetNodeByOwner(req.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	client := &model.Client{
		Name:          req.Name,
		NodeID:        req.NodeID,
		LocalPort:     req.LocalPort,
		RemotePort:    req.RemotePort,
		ProxyUser:     req.ProxyUser,
		ProxyPass:     req.ProxyPass,
		TrafficQuota:  req.TrafficQuota,
		QuotaResetDay: req.QuotaResetDay,
		OwnerID:       &userID,
	}

	if client.LocalPort == 0 {
		client.LocalPort = 38777
	}
	if client.QuotaResetDay == 0 {
		client.QuotaResetDay = 1
	}

	if err := s.svc.CreateClient(client); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, client)
}

func (s *Server) updateClient(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetClientByOwner(id, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delete(updates, "id")
	delete(updates, "token")
	delete(updates, "created_at")
	delete(updates, "owner_id")
	if !isAdmin {
		if rawNodeID, exists := updates["node_id"]; exists {
			if rawNodeID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
				return
			}
			nodeID, err := parseUintField(rawNodeID, "node_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			allowed, msg := s.svc.CheckPlanNodeAccess(userID, nodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(nodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	// 密码字段为空时不更新，防止编辑时误覆盖已有密码
	if v, ok := updates["proxy_pass"]; ok && v == "" {
		delete(updates, "proxy_pass")
	}

	if err := s.svc.UpdateClient(id, updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteClient(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetClientByOwner(id, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := s.svc.DeleteClient(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getClientInstallScript(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	osType := c.DefaultQuery("os", "linux")
	userID, isAdmin := getUserInfo(c)

	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	// 获取正确的 Panel URL（支持反向代理）
	panelURL := s.getPanelURL(c)
	githubRaw := s.cfg.GitHubRawURL

	var script, oneLineCommand string

	if osType == "windows" {
		// PowerShell 安装命令
		oneLineCommand = fmt.Sprintf(`irm "%s/install-client.ps1" -OutFile "$env:TEMP\install-client.ps1"; & "$env:TEMP\install-client.ps1" -PanelUrl "%s" -Token "%s"`, githubRaw, panelURL, client.Token)
		script = fmt.Sprintf(`# GOST Panel Client Installation for Windows
# Run in PowerShell as Administrator

# Client: %s
# Local SOCKS5 Port: %d (credentials configured in panel)

# One-line install (copy and run this command):
%s
`, client.Name, client.LocalPort, oneLineCommand)
	} else {
		// Bash 安装命令
		oneLineCommand = fmt.Sprintf(`(curl -fsSL "%s/install-client.sh" 2>/dev/null || wget -qO- "%s/install-client.sh") | bash -s -- -p "%s" -t "%s"`, githubRaw, githubRaw, panelURL, client.Token)
		script = fmt.Sprintf(`#!/bin/bash
# GOST Panel Client Installation (Agent Mode)
# Supported: Linux (amd64, arm64, armv7, armv6, mips, mipsle)

# Client: %s
# Local SOCKS5 Port: %d (credentials configured in panel)

# One-line install (copy and run this command):
%s

# Or with forced architecture:
# (curl -fsSL "%s/install-client.sh" 2>/dev/null || wget -qO- "%s/install-client.sh") | bash -s -- -p "%s" -t "%s" -a armv6
`, client.Name, client.LocalPort, oneLineCommand, githubRaw, githubRaw, panelURL, client.Token)
	}

	c.JSON(http.StatusOK, gin.H{
		"script":           script,
		"one_line_command": oneLineCommand,
		"token":            client.Token,
	})
}

func (s *Server) getClientGostConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	config := s.generateClientConfig(client)
	c.YAML(http.StatusOK, config)
}

func (s *Server) getClientProxyURI(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	client, err := s.svc.GetClientByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	// 生成通过节点访问的代理 URI
	// 格式: socks5://user:pass@节点地址:远程端口
	if client.Node == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "客户端未关联节点"})
		return
	}
	node := client.Node
	uri := fmt.Sprintf("socks5://%s:%s@%s:%d",
		client.ProxyUser, client.ProxyPass, node.Host, client.RemotePort)

	c.JSON(http.StatusOK, gin.H{"uri": uri})
}

func (s *Server) generateClientConfig(client *model.Client) map[string]interface{} {
	node := client.Node
	chainName := "forward-chain"

	config := map[string]interface{}{
		"services": []map[string]interface{}{
			// 本地 SOCKS5
			{
				"name": "local-socks5",
				"addr": fmt.Sprintf(":%d", client.LocalPort),
				"handler": map[string]interface{}{
					"type":   "socks5",
					"auther": "local-auth",
					"metadata": map[string]interface{}{
						"udp":         true,
						"udpAddr":     fmt.Sprintf(":%d", client.LocalPort),
						"ignoreChain": true,
					},
				},
				"listener": map[string]interface{}{
					"type": "tcp",
				},
			},
			// RTCP 反向隧道
			{
				"name": "rtcp-tunnel",
				"addr": fmt.Sprintf(":%d", client.RemotePort),
				"handler": map[string]interface{}{
					"type": "rtcp",
				},
				"listener": map[string]interface{}{
					"type":  "rtcp",
					"chain": chainName,
					"metadata": map[string]interface{}{
						"keepalive": true,
					},
				},
				"forwarder": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{"name": "target", "addr": fmt.Sprintf("127.0.0.1:%d", client.LocalPort)},
					},
				},
			},
			// RUDP 反向隧道
			{
				"name": "rudp-tunnel",
				"addr": fmt.Sprintf(":%d", client.RemotePort),
				"handler": map[string]interface{}{
					"type": "rudp",
				},
				"listener": map[string]interface{}{
					"type":  "rudp",
					"chain": chainName,
					"metadata": map[string]interface{}{
						"keepalive": true,
					},
				},
				"forwarder": map[string]interface{}{
					"nodes": []map[string]interface{}{
						{"name": "target", "addr": fmt.Sprintf("127.0.0.1:%d", client.LocalPort)},
					},
				},
			},
		},
		"chains": []map[string]interface{}{
			{
				"name": chainName,
				"hops": []map[string]interface{}{
					{
						"name": "hop-0",
						"nodes": []map[string]interface{}{
							{
								"name": "node-0",
								// 连接到节点的 SOCKS5 服务 (使用 bind 功能建立反向隧道)
								"addr": fmt.Sprintf("%s:%d", node.Host, node.Port),
								"connector": func() map[string]interface{} {
									c := map[string]interface{}{
										"type": "socks5",
									}
									// 如果节点有代理认证，添加认证信息
									if node.ProxyUser != "" {
										c["auth"] = map[string]string{
											"username": node.ProxyUser,
											"password": node.ProxyPass,
										}
									}
									return c
								}(),
								"dialer": map[string]interface{}{
									"type": "tcp",
									"metadata": map[string]interface{}{
										"keepAlive":       true,
										"keepAlivePeriod": "15s",
									},
								},
							},
						},
					},
				},
			},
		},
		"authers": []map[string]interface{}{
			{
				"name": "local-auth",
				"auths": []map[string]string{
					{"username": client.ProxyUser, "password": client.ProxyPass},
				},
			},
		},
	}

	return config
}

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

	// Token 无效，通知 Agent 卸载自己
	c.JSON(http.StatusUnauthorized, gin.H{
		"error":     "invalid token",
		"uninstall": true,
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

// ==================== 用户管理 ====================

func (s *Server) listUsers(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	users, err := s.svc.GetUsersWithTrafficSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (s *Server) getUser(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	user, err := s.svc.GetUser(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, user)
}

type CreateUserRequest struct {
	Username      string `json:"username" binding:"required"`
	Email         string `json:"email"`
	Password      string `json:"password" binding:"required"`
	Role          string `json:"role"`
	Enabled       *bool  `json:"enabled"` // 使用指针以区分未设置和 false
	EmailVerified *bool  `json:"email_verified"`
}

func (s *Server) createUser(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 默认启用账户
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// 管理员创建的账户默认验证邮箱
	emailVerified := true
	if req.EmailVerified != nil {
		emailVerified = *req.EmailVerified
	}

	user, err := s.svc.CreateUserFull(req.Username, req.Email, req.Password, req.Role, enabled, emailVerified)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

func (s *Server) updateUser(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 防止篡改敏感字段
	delete(updates, "id")
	delete(updates, "created_at")

	if err := s.svc.UpdateUser(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteUser(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.DeleteUser(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

func (s *Server) changePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 从 JWT 获取当前用户 ID
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	id := uint(userIDFloat)
	if err := s.svc.ChangePassword(id, req.OldPassword, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if currentJTI, ok := c.Get("jti"); ok {
		if jti, ok := currentJTI.(string); ok && jti != "" {
			_, _ = s.svc.DeleteOtherSessions(id, jti)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 个人账户设置 ====================

// getProfile 获取当前用户个人资料
func (s *Server) getProfile(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	user, err := s.svc.GetUser(uint(userIDFloat))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateProfileRequest 更新个人资料请求
type UpdateProfileRequest struct {
	Email *string `json:"email"`
}

// updateProfile 更新当前用户个人资料
func (s *Server) updateProfile(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := make(map[string]interface{})
	if req.Email != nil {
		updates["email"] = *req.Email
		// 如果邮箱变更，需要重新验证
		updates["email_verified"] = false
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no updates provided"})
		return
	}

	if err := s.svc.UpdateUser(uint(userIDFloat), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回更新后的用户信息
	user, _ := s.svc.GetUser(uint(userIDFloat))
	c.JSON(http.StatusOK, user)
}

// ==================== 2FA 双因素认证 ====================

// enable2FA 开始启用 2FA
func (s *Server) enable2FA(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	userID := uint(userIDFloat)

	var user model.User
	if err := s.svc.DB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	if user.TwoFactorEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "2FA 已启用"})
		return
	}

	secret, qrCode, err := s.svc.GenerateTOTPSecret(user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成密钥失败"})
		return
	}

	// 临时保存 secret（未激活状态）
	s.svc.DB().Model(&user).Update("two_factor_secret", secret)

	c.JSON(http.StatusOK, gin.H{
		"secret": secret,
		"qrcode": qrCode,
	})
}

// verify2FA 验证并正式启用 2FA
func (s *Server) verify2FA(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	userID := uint(userIDFloat)

	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入验证码"})
		return
	}

	var user model.User
	if err := s.svc.DB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	if !s.svc.ValidateTOTP(user.TwoFactorSecret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证码错误"})
		return
	}

	// 生成备份码
	codes, hashJSON, err := s.svc.GenerateBackupCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成备份码失败"})
		return
	}

	s.svc.DB().Model(&user).Updates(map[string]interface{}{
		"two_factor_enabled": true,
		"backup_codes":       hashJSON,
	})

	// 记录操作日志
	username, _ := c.Get("username")
	s.svc.LogOperation(userID, username.(string), "enable", "2fa", userID, "启用 2FA", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{
		"backup_codes": codes,
		"message":      "2FA 已启用",
	})
}

// disable2FA 禁用 2FA
func (s *Server) disable2FA(c *gin.Context) {
	userIDRaw, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userIDFloat, ok := userIDRaw.(float64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}
	userID := uint(userIDFloat)

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入密码"})
		return
	}

	var user model.User
	if err := s.svc.DB().First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	// 验证密码
	if !model.CheckPassword(user.Password, req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码错误"})
		return
	}

	s.svc.DB().Model(&user).Updates(map[string]interface{}{
		"two_factor_enabled": false,
		"two_factor_secret":  "",
		"backup_codes":       "",
	})

	// 记录操作日志
	username, _ := c.Get("username")
	s.svc.LogOperation(userID, username.(string), "disable", "2fa", userID, "禁用 2FA", c.ClientIP(), c.GetHeader("User-Agent"), "success")

	c.JSON(http.StatusOK, gin.H{"message": "2FA 已禁用"})
}

// ==================== 流量历史 ====================

func (s *Server) getTrafficHistory(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	hoursStr := c.DefaultQuery("hours", "1")
	hours, _ := strconv.Atoi(hoursStr)

	nodeIDStr := c.Query("node_id")
	var nodeID *uint
	if nodeIDStr != "" {
		id, err := strconv.ParseUint(nodeIDStr, 10, 32)
		if err == nil {
			uid := uint(id)
			// Verify ownership of the node
			if _, err := s.svc.GetNodeByOwner(uid, userID, isAdmin); err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "节点不存在"})
				return
			}
			nodeID = &uid
		}
	} else if !isAdmin {
		// Non-admin without node_id: only show their own nodes' traffic
		// For simplicity, return empty if no node_id specified for non-admin
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	history, err := s.svc.GetTrafficHistory(nodeID, hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, history)
}

// ==================== 通知渠道管理 ====================

func (s *Server) listNotifyChannels(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	channels, err := s.svc.GetAlertService().ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, channels)
}

func (s *Server) getNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	channel, err := s.svc.GetAlertService().GetChannel(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}
	c.JSON(http.StatusOK, channel)
}

type CreateNotifyChannelRequest struct {
	Name    string                 `json:"name" binding:"required"`
	Type    string                 `json:"type" binding:"required"` // telegram/webhook/smtp
	Config  map[string]interface{} `json:"config" binding:"required"`
	Enabled bool                   `json:"enabled"`
}

func (s *Server) createNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req CreateNotifyChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 将 config 对象转为 JSON 字符串
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
		return
	}

	channel := &model.NotifyChannel{
		Name:    req.Name,
		Type:    req.Type,
		Config:  string(configJSON),
		Enabled: req.Enabled,
	}

	if err := s.svc.GetAlertService().CreateChannel(channel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, channel)
}

func (s *Server) updateNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delete(updates, "id")
	delete(updates, "created_at")

	// 如果 config 是对象，转为 JSON 字符串
	if config, ok := updates["config"]; ok && config != nil {
		if configMap, isMap := config.(map[string]interface{}); isMap {
			configJSON, err := json.Marshal(configMap)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config format"})
				return
			}
			updates["config"] = string(configJSON)
		}
	}

	if err := s.svc.GetAlertService().UpdateChannel(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.GetAlertService().DeleteChannel(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) testNotifyChannel(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.GetAlertService().TestChannel(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试通知已发送"})
}

// ==================== 告警规则管理 ====================

func (s *Server) listAlertRules(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	rules, err := s.svc.GetAlertService().ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 转换为前端期望的格式
	formattedRules := s.formatAlertRules(rules)
	c.JSON(http.StatusOK, formattedRules)
}

func (s *Server) getAlertRule(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	rule, err := s.svc.GetAlertService().GetRule(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	// 转换为前端期望的格式
	formatted := s.formatAlertRule(rule)
	c.JSON(http.StatusOK, formatted)
}

// formatAlertRules 转换告警规则列表为前端格式
func (s *Server) formatAlertRules(rules []model.AlertRule) []gin.H {
	result := make([]gin.H, len(rules))
	for i, rule := range rules {
		result[i] = s.formatAlertRule(&rule)
	}
	return result
}

// formatAlertRule 转换单个告警规则为前端格式
func (s *Server) formatAlertRule(rule *model.AlertRule) gin.H {
	// 解析 channel_ids 字符串为数组 - 始终返回数组，即使为空
	channelIDs := make([]int, 0)
	if rule.ChannelIDs != "" {
		for _, idStr := range strings.Split(rule.ChannelIDs, ",") {
			idStr = strings.TrimSpace(idStr)
			if id, err := strconv.Atoi(idStr); err == nil {
				channelIDs = append(channelIDs, id)
			}
		}
	}

	// 解析 condition JSON 字符串为对象
	var condition interface{}
	if rule.Condition != "" && rule.Condition != "{}" {
		json.Unmarshal([]byte(rule.Condition), &condition)
	} else {
		condition = map[string]interface{}{}
	}

	return gin.H{
		"id":               rule.ID,
		"name":             rule.Name,
		"type":             rule.Type,
		"alert_type":       rule.Type, // 前端兼容字段
		"condition":        condition,
		"channel_ids":      channelIDs,
		"enabled":          rule.Enabled,
		"cooldown_min":     rule.CooldownMin,
		"silence_duration": rule.CooldownMin * 60000, // 转为毫秒给前端
		"last_alert_at":    rule.LastAlertAt,
		"created_at":       rule.CreatedAt,
		"updated_at":       rule.UpdatedAt,
	}
}

type CreateAlertRuleRequest struct {
	Name            string      `json:"name" binding:"required"`
	Type            string      `json:"type"`        // 后端字段名
	AlertType       string      `json:"alert_type"`  // 前端字段名 (兼容)
	Condition       interface{} `json:"condition"`   // 接受对象或字符串
	ChannelIDs      interface{} `json:"channel_ids"` // 接受数组或字符串
	Enabled         bool        `json:"enabled"`
	CooldownMin     int         `json:"cooldown_min"`     // 后端字段名 (分钟)
	SilenceDuration int         `json:"silence_duration"` // 前端字段名 (毫秒，兼容)
}

func (s *Server) createAlertRule(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var req CreateAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 兼容前端字段名
	ruleType := req.Type
	if ruleType == "" {
		ruleType = req.AlertType
	}
	if ruleType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type or alert_type is required"})
		return
	}

	// 处理 condition (对象转字符串)
	conditionStr := ""
	if req.Condition != nil {
		switch v := req.Condition.(type) {
		case string:
			conditionStr = v
		case map[string]interface{}:
			if data, err := json.Marshal(v); err == nil {
				conditionStr = string(data)
			}
		}
	}

	// 处理 channel_ids (数组转逗号分隔字符串)
	channelIDsStr := ""
	if req.ChannelIDs != nil {
		switch v := req.ChannelIDs.(type) {
		case string:
			channelIDsStr = v
		case []interface{}:
			ids := make([]string, len(v))
			for i, id := range v {
				ids[i] = fmt.Sprintf("%v", id)
			}
			channelIDsStr = strings.Join(ids, ",")
		}
	}

	// 处理 cooldown_min (前端发毫秒，转为分钟)
	cooldownMin := req.CooldownMin
	if cooldownMin == 0 && req.SilenceDuration > 0 {
		cooldownMin = req.SilenceDuration / 60000 // 毫秒转分钟
	}

	rule := &model.AlertRule{
		Name:        req.Name,
		Type:        ruleType,
		Condition:   conditionStr,
		ChannelIDs:  channelIDsStr,
		Enabled:     req.Enabled,
		CooldownMin: cooldownMin,
	}

	if rule.CooldownMin == 0 {
		rule.CooldownMin = 30
	}

	if err := s.svc.GetAlertService().CreateRule(rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rule)
}

func (s *Server) updateAlertRule(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delete(updates, "id")
	delete(updates, "created_at")
	delete(updates, "last_alert_at")

	// 兼容前端字段名: alert_type -> type
	if alertType, ok := updates["alert_type"]; ok {
		updates["type"] = alertType
		delete(updates, "alert_type")
	}

	// 兼容前端字段名: silence_duration (毫秒) -> cooldown_min (分钟)
	if silenceDuration, ok := updates["silence_duration"]; ok {
		if ms, isFloat := silenceDuration.(float64); isFloat {
			updates["cooldown_min"] = int(ms / 60000)
		}
		delete(updates, "silence_duration")
	}

	// 处理 condition (对象转字符串)
	if condition, ok := updates["condition"]; ok && condition != nil {
		if condMap, isMap := condition.(map[string]interface{}); isMap {
			if data, err := json.Marshal(condMap); err == nil {
				updates["condition"] = string(data)
			}
		}
	}

	// 处理 channel_ids (数组转逗号分隔字符串)
	if channelIDs, ok := updates["channel_ids"]; ok && channelIDs != nil {
		if ids, isArray := channelIDs.([]interface{}); isArray {
			strIDs := make([]string, len(ids))
			for i, id := range ids {
				strIDs[i] = fmt.Sprintf("%v", id)
			}
			updates["channel_ids"] = strings.Join(strIDs, ",")
		}
	}

	if err := s.svc.GetAlertService().UpdateRule(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteAlertRule(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.GetAlertService().DeleteRule(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 告警日志 ====================

func (s *Server) getAlertLogs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if limit <= 0 || limit > 100 {
		limit = 50
	}

	logs, total, err := s.svc.GetAlertService().GetAlertLogs(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 转换为前端期望的格式
	formattedLogs := make([]gin.H, len(logs))
	for i, log := range logs {
		formattedLogs[i] = gin.H{
			"id":          log.ID,
			"rule_id":     log.RuleID,
			"rule_name":   log.RuleName,
			"type":        log.Type,
			"alert_type":  log.Type, // 前端兼容字段
			"message":     log.Message,
			"target_type": log.TargetType,
			"target_id":   log.TargetID,
			"target_name": log.TargetName,
			"status":      log.Status,
			"sent":        log.Status == "sent", // 前端期望的布尔值
			"created_at":  log.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  formattedLogs,
		"total": total,
	})
}

// ==================== 端口转发 ====================

// portForwardToResponse 转换 PortForward 为前端兼容的响应格式
func portForwardToResponse(pf model.PortForward, nodeName string) gin.H {
	listenHost := "0.0.0.0"
	listenPort := 0
	if pf.LocalAddr != "" {
		if h, p, err := net.SplitHostPort(pf.LocalAddr); err == nil {
			if h != "" {
				listenHost = h
			}
			listenPort, _ = strconv.Atoi(p)
		}
	}

	targetHost := ""
	targetPort := 0
	if pf.RemoteAddr != "" {
		if h, p, err := net.SplitHostPort(pf.RemoteAddr); err == nil {
			targetHost = h
			targetPort, _ = strconv.Atoi(p)
		}
	}

	return gin.H{
		"id":          pf.ID,
		"node_id":     pf.NodeID,
		"name":        pf.Name,
		"type":        pf.Type,
		"protocol":    pf.Type,
		"local_addr":  pf.LocalAddr,
		"remote_addr": pf.RemoteAddr,
		"listen_host": listenHost,
		"listen_port": listenPort,
		"target_host": targetHost,
		"target_port": targetPort,
		"chain_id":    pf.ChainID,
		"enabled":     pf.Enabled,
		"owner_id":    pf.OwnerID,
		"node_name":   nodeName,
		"created_at":  pf.CreatedAt,
		"updated_at":  pf.UpdatedAt,
	}
}

func (s *Server) listPortForwards(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	forwards, err := s.svc.ListPortForwards(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 构建节点名称映射
	nodeIDs := make(map[uint]bool)
	for _, f := range forwards {
		if f.NodeID > 0 {
			nodeIDs[f.NodeID] = true
		}
	}
	nodeNames := make(map[uint]string)
	if len(nodeIDs) > 0 {
		nodes, _ := s.svc.ListNodesByOwner(userID, isAdmin)
		for _, n := range nodes {
			if nodeIDs[n.ID] {
				nodeNames[n.ID] = n.Name
			}
		}
	}

	result := make([]gin.H, len(forwards))
	for i, f := range forwards {
		result[i] = portForwardToResponse(f, nodeNames[f.NodeID])
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) getPortForward(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	forward, err := s.svc.GetPortForwardByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "port forward not found"})
		return
	}

	nodeName := ""
	if forward.NodeID > 0 {
		if node, err := s.svc.GetNodeByOwner(forward.NodeID, userID, isAdmin); err == nil {
			nodeName = node.Name
		}
	}
	c.JSON(http.StatusOK, portForwardToResponse(*forward, nodeName))
}

type CreatePortForwardRequest struct {
	NodeID     uint   `json:"node_id"`
	Name       string `json:"name" binding:"required"`
	Type       string `json:"type"`        // tcp/udp/rtcp/rudp (后端字段)
	Protocol   string `json:"protocol"`    // 前端字段 (兼容)
	LocalAddr  string `json:"local_addr"`  // 后端字段
	RemoteAddr string `json:"remote_addr"` // 后端字段
	// 前端兼容字段
	ListenHost  string `json:"listen_host"`
	ListenPort  int    `json:"listen_port"`
	TargetHost  string `json:"target_host"`
	TargetPort  int    `json:"target_port"`
	Description string `json:"description"` // 前端发送但后端忽略
	ChainID     *uint  `json:"chain_id"`
	Enabled     bool   `json:"enabled"`
}

func (s *Server) createPortForward(c *gin.Context) {
	var req CreatePortForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 兼容前端字段
	forwardType := req.Type
	if forwardType == "" {
		forwardType = req.Protocol
	}
	if forwardType == "" {
		forwardType = "tcp"
	}

	localAddr := req.LocalAddr
	if localAddr == "" && req.ListenPort > 0 {
		host := req.ListenHost
		if host == "" {
			host = "0.0.0.0"
		}
		localAddr = fmt.Sprintf("%s:%d", host, req.ListenPort)
	}

	remoteAddr := req.RemoteAddr
	if remoteAddr == "" && req.TargetPort > 0 {
		remoteAddr = fmt.Sprintf("%s:%d", req.TargetHost, req.TargetPort)
	}

	if localAddr == "" || remoteAddr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "local_addr and remote_addr are required"})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "port_forward")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		// 检查节点访问权限
		if req.NodeID > 0 {
			allowed, msg = s.svc.CheckPlanNodeAccess(userID, req.NodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(req.NodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
		if req.ChainID != nil {
			if _, err := s.svc.GetProxyChainByOwner(*req.ChainID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	forward := &model.PortForward{
		NodeID:     req.NodeID,
		Name:       req.Name,
		Type:       forwardType,
		LocalAddr:  localAddr,
		RemoteAddr: remoteAddr,
		ChainID:    req.ChainID,
		Enabled:    req.Enabled,
		OwnerID:    &userID,
	}

	if err := s.svc.CreatePortForward(forward); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, forward)
}

func (s *Server) updatePortForward(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetPortForwardByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此转发规则"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "created_at")
	delete(updates, "updated_at")
	delete(updates, "description") // 前端发送但后端不支持
	delete(updates, "node_name")   // 响应专用字段

	// 兼容前端字段: protocol -> type
	if protocol, ok := updates["protocol"]; ok {
		updates["type"] = protocol
		delete(updates, "protocol")
	}

	// 兼容前端字段: listen_host + listen_port -> local_addr
	if _, hasListenPort := updates["listen_port"]; hasListenPort {
		host := "0.0.0.0"
		if h, ok := updates["listen_host"].(string); ok && h != "" {
			host = h
		}
		port := int(updates["listen_port"].(float64))
		updates["local_addr"] = fmt.Sprintf("%s:%d", host, port)
		delete(updates, "listen_host")
		delete(updates, "listen_port")
	}

	// 兼容前端字段: target_host + target_port -> remote_addr
	if _, hasTargetPort := updates["target_port"]; hasTargetPort {
		host := ""
		if h, ok := updates["target_host"].(string); ok {
			host = h
		}
		port := int(updates["target_port"].(float64))
		updates["remote_addr"] = fmt.Sprintf("%s:%d", host, port)
		delete(updates, "target_host")
		delete(updates, "target_port")
	}

	if !isAdmin {
		if rawNodeID, exists := updates["node_id"]; exists {
			if rawNodeID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid node_id"})
				return
			}
			nodeID, err := parseUintField(rawNodeID, "node_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			allowed, msg := s.svc.CheckPlanNodeAccess(userID, nodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(nodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
		if rawChainID, exists := updates["chain_id"]; exists && rawChainID != nil {
			chainID, err := parseUintField(rawChainID, "chain_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			if _, err := s.svc.GetProxyChainByOwner(chainID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	if err := s.svc.UpdatePortForward(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deletePortForward(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetPortForwardByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此转发规则"})
		return
	}

	if err := s.svc.DeletePortForward(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 节点组 (负载均衡) ====================

func (s *Server) listNodeGroups(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	groups, err := s.svc.ListNodeGroups(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func (s *Server) getNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	group, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node group not found"})
		return
	}
	c.JSON(http.StatusOK, group)
}

type CreateNodeGroupRequest struct {
	Name          string `json:"name" binding:"required"`
	Strategy      string `json:"strategy"`       // round/random/fifo/hash (后端)
	Selector      string `json:"selector"`       // 选择器配置 JSON
	FailTimeout   int    `json:"fail_timeout"`   // 故障超时时间(秒)
	MaxFails      int    `json:"max_fails"`      // 最大失败次数
	HealthCheck   bool   `json:"health_check"`   // 后端字段
	CheckInterval int    `json:"check_interval"` // 健康检查间隔(秒)
	// 前端兼容字段
	HealthCheckEnabled  bool   `json:"health_check_enabled"`  // 前端字段
	HealthCheckInterval int    `json:"health_check_interval"` // 前端字段 (毫秒)
	HealthCheckTimeout  int    `json:"health_check_timeout"`  // 前端发送但忽略
	Description         string `json:"description"`           // 前端发送但忽略
}

func (s *Server) createNodeGroup(c *gin.Context) {
	var req CreateNodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "node_group")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}
	}

	// 兼容前端策略值
	strategy := req.Strategy
	if strategy == "round_robin" {
		strategy = "round"
	}
	if strategy == "" {
		strategy = "round"
	}

	// 兼容前端健康检查字段
	healthCheck := req.HealthCheck || req.HealthCheckEnabled
	checkInterval := req.CheckInterval
	if checkInterval == 0 && req.HealthCheckInterval > 0 {
		checkInterval = req.HealthCheckInterval / 1000 // 毫秒转秒
	}
	if checkInterval == 0 {
		checkInterval = 30
	}

	group := &model.NodeGroup{
		Name:          req.Name,
		Strategy:      strategy,
		Selector:      req.Selector,
		FailTimeout:   req.FailTimeout,
		MaxFails:      req.MaxFails,
		HealthCheck:   healthCheck,
		CheckInterval: checkInterval,
		OwnerID:       &userID,
	}

	// 默认值
	if group.FailTimeout == 0 {
		group.FailTimeout = 30
	}
	if group.MaxFails == 0 {
		group.MaxFails = 3
	}

	if err := s.svc.CreateNodeGroup(group); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, group)
}

func (s *Server) updateNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "created_at")
	delete(updates, "description")          // 前端发送但不支持
	delete(updates, "health_check_timeout") // 前端发送但不支持

	// 兼容前端策略值: round_robin -> round
	if strategy, ok := updates["strategy"].(string); ok && strategy == "round_robin" {
		updates["strategy"] = "round"
	}

	// 兼容前端字段: health_check_enabled -> health_check
	if healthCheckEnabled, ok := updates["health_check_enabled"]; ok {
		updates["health_check"] = healthCheckEnabled
		delete(updates, "health_check_enabled")
	}

	// 兼容前端字段: health_check_interval (毫秒) -> check_interval (秒)
	if interval, ok := updates["health_check_interval"].(float64); ok {
		updates["check_interval"] = int(interval / 1000)
		delete(updates, "health_check_interval")
	}

	if err := s.svc.UpdateNodeGroup(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteNodeGroup(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}

	if err := s.svc.DeleteNodeGroup(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// 节点组成员

func (s *Server) listNodeGroupMembers(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}
	members, err := s.svc.ListNodeGroupMembers(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, members)
}

type AddNodeGroupMemberRequest struct {
	NodeID   uint `json:"node_id" binding:"required"`
	Weight   int  `json:"weight"`
	Priority int  `json:"priority"`
}

func (s *Server) addNodeGroupMember(c *gin.Context) {
	groupID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeGroupByOwner(uint(groupID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}

	var req AddNodeGroupMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(req.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	member := &model.NodeGroupMember{
		GroupID:  uint(groupID),
		NodeID:   req.NodeID,
		Weight:   req.Weight,
		Priority: req.Priority,
		Enabled:  true,
	}

	if member.Weight == 0 {
		member.Weight = 1
	}

	if err := s.svc.AddNodeGroupMember(member); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, member)
}

func (s *Server) removeNodeGroupMember(c *gin.Context) {
	groupID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeGroupByOwner(uint(groupID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点组"})
		return
	}
	memberID, _ := strconv.ParseUint(c.Param("memberId"), 10, 32)
	if err := s.svc.DB().Where("id = ? AND group_id = ?", uint(memberID), uint(groupID)).First(&model.NodeGroupMember{}).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}

	if err := s.svc.RemoveNodeGroupMember(uint(memberID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getNodeGroupConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	group, err := s.svc.GetNodeGroupByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node group not found"})
		return
	}

	members, err := s.svc.GetNodeGroupMembersWithNodes(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateChainConfig(group, members)

	c.YAML(http.StatusOK, config)
}

// ==================== 操作日志 ====================

func (s *Server) getOperationLogs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	action := c.Query("action")
	resource := c.Query("resource")

	if limit > 100 {
		limit = 100
	}

	logs, total, err := s.svc.GetOperationLogs(limit, offset, action, resource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": total,
	})
}

func (s *Server) getNodeProxyURI(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	uri := gost.GenerateProxyURI(node)
	c.JSON(http.StatusOK, gin.H{"uri": uri})
}

// pingNode 测试节点延迟
func (s *Server) pingNode(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 测试 TCP 连接延迟到节点的代理端口
	addr := fmt.Sprintf("%s:%d", node.Host, node.Port)
	start := time.Now()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"latency": -1,
			"error":   "connection failed",
		})
		return
	}
	conn.Close()

	latency := time.Since(start).Milliseconds()
	c.JSON(http.StatusOK, gin.H{
		"latency": latency,
	})
}

// pingAllNodes 批量测试所有节点延迟（并发执行）
func (s *Server) pingAllNodes(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	nodes, err := s.svc.ListNodesByOwner(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	results := make(map[uint]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 限制并发数，避免过多连接
	semaphore := make(chan struct{}, 20)

	for _, node := range nodes {
		wg.Add(1)
		go func(n model.Node) {
			defer wg.Done()
			semaphore <- struct{}{}        // 获取信号量
			defer func() { <-semaphore }() // 释放信号量

			addr := fmt.Sprintf("%s:%d", n.Host, n.Port)
			start := time.Now()

			conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
			var latency int64 = -1
			if err == nil {
				conn.Close()
				latency = time.Since(start).Milliseconds()
			}

			mu.Lock()
			results[n.ID] = latency
			mu.Unlock()
		}(node)
	}

	wg.Wait()
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// getNodeHealthLogs 获取节点健康检查日志
func (s *Server) getNodeHealthLogs(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	userID, isAdmin := getUserInfo(c)

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if _, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	var logs []model.HealthCheckLog
	if err := s.svc.DB().Where("node_id = ?", uint(id)).
		Order("checked_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// getHealthSummary 获取健康检查概览
func (s *Server) getHealthSummary(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	nodes, err := s.svc.ListNodesByOwner(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type NodeHealthSummary struct {
		NodeID      uint      `json:"node_id"`
		NodeName    string    `json:"node_name"`
		Status      string    `json:"status"`
		LastCheck   time.Time `json:"last_check"`
		AvgLatency  int       `json:"avg_latency"`
		FailureRate float64   `json:"failure_rate"`
	}

	summaries := make([]NodeHealthSummary, 0, len(nodes))

	for _, node := range nodes {
		var lastLog model.HealthCheckLog
		var totalChecks int64
		var failedChecks int64
		var avgLatency float64

		// 获取最近一次健康检查
		s.svc.DB().Where("node_id = ?", node.ID).
			Order("checked_at DESC").
			First(&lastLog)

		// 统计最近24小时的健康检查
		since := time.Now().Add(-24 * time.Hour)
		s.svc.DB().Model(&model.HealthCheckLog{}).
			Where("node_id = ? AND checked_at >= ?", node.ID, since).
			Count(&totalChecks)

		s.svc.DB().Model(&model.HealthCheckLog{}).
			Where("node_id = ? AND checked_at >= ? AND status = ?", node.ID, since, "unhealthy").
			Count(&failedChecks)

		// 计算平均延迟（仅健康检查）
		var result struct {
			AvgLatency float64
		}
		s.svc.DB().Model(&model.HealthCheckLog{}).
			Select("AVG(latency) as avg_latency").
			Where("node_id = ? AND checked_at >= ? AND status = ?", node.ID, since, "healthy").
			Scan(&result)
		avgLatency = result.AvgLatency

		failureRate := 0.0
		if totalChecks > 0 {
			failureRate = float64(failedChecks) / float64(totalChecks) * 100
		}

		summaries = append(summaries, NodeHealthSummary{
			NodeID:      node.ID,
			NodeName:    node.Name,
			Status:      node.Status,
			LastCheck:   lastLog.CheckedAt,
			AvgLatency:  int(avgLatency),
			FailureRate: failureRate,
		})
	}

	c.JSON(http.StatusOK, gin.H{"summaries": summaries})
}

// ==================== 数据导出 ====================

// ExportNode 节点导出结构
type ExportNode struct {
	Name       string `json:"name" yaml:"name"`
	Host       string `json:"host" yaml:"host"`
	Port       int    `json:"port" yaml:"port"`
	Protocol   string `json:"protocol" yaml:"protocol"`
	Transport  string `json:"transport" yaml:"transport"`
	ProxyUser  string `json:"proxy_user,omitempty" yaml:"proxy_user,omitempty"`
	ProxyPass  string `json:"proxy_pass,omitempty" yaml:"proxy_pass,omitempty"`
	SSMethod   string `json:"ss_method,omitempty" yaml:"ss_method,omitempty"`
	SSPassword string `json:"ss_password,omitempty" yaml:"ss_password,omitempty"`
	TLSEnabled bool   `json:"tls_enabled" yaml:"tls_enabled"`
	TLSSNI     string `json:"tls_sni,omitempty" yaml:"tls_sni,omitempty"`
	WSPath     string `json:"ws_path,omitempty" yaml:"ws_path,omitempty"`
	WSHost     string `json:"ws_host,omitempty" yaml:"ws_host,omitempty"`
}

// ExportClient 客户端导出结构
type ExportClient struct {
	Name       string `json:"name" yaml:"name"`
	NodeName   string `json:"node_name" yaml:"node_name"`
	LocalPort  int    `json:"local_port" yaml:"local_port"`
	RemotePort int    `json:"remote_port" yaml:"remote_port"`
	ProxyUser  string `json:"proxy_user,omitempty" yaml:"proxy_user,omitempty"`
	ProxyPass  string `json:"proxy_pass,omitempty" yaml:"proxy_pass,omitempty"`
}

// ExportData 导出数据结构
type ExportData struct {
	Version  string         `json:"version" yaml:"version"`
	ExportAt string         `json:"export_at" yaml:"export_at"`
	Nodes    []ExportNode   `json:"nodes,omitempty" yaml:"nodes,omitempty"`
	Clients  []ExportClient `json:"clients,omitempty" yaml:"clients,omitempty"`
}

// exportData 导出数据
func (s *Server) exportData(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	format := c.DefaultQuery("format", "json")
	dataType := c.DefaultQuery("type", "all") // all, nodes, clients

	exportData := ExportData{
		Version:  "1.0",
		ExportAt: time.Now().Format(time.RFC3339),
	}

	// 导出节点
	if dataType == "all" || dataType == "nodes" {
		nodes, err := s.svc.ListNodes()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, n := range nodes {
			exportData.Nodes = append(exportData.Nodes, ExportNode{
				Name:       n.Name,
				Host:       n.Host,
				Port:       n.Port,
				Protocol:   n.Protocol,
				Transport:  n.Transport,
				ProxyUser:  n.ProxyUser,
				ProxyPass:  n.ProxyPass,
				SSMethod:   n.SSMethod,
				SSPassword: n.SSPassword,
				TLSEnabled: n.TLSEnabled,
				TLSSNI:     n.TLSSNI,
				WSPath:     n.WSPath,
				WSHost:     n.WSHost,
			})
		}
	}

	// 导出客户端
	if dataType == "all" || dataType == "clients" {
		clients, err := s.svc.ListClients()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, cl := range clients {
			nodeName := ""
			if cl.Node != nil {
				nodeName = cl.Node.Name
			}
			exportData.Clients = append(exportData.Clients, ExportClient{
				Name:       cl.Name,
				NodeName:   nodeName,
				LocalPort:  cl.LocalPort,
				RemotePort: cl.RemotePort,
				ProxyUser:  cl.ProxyUser,
				ProxyPass:  cl.ProxyPass,
			})
		}
	}

	// 输出格式
	if format == "yaml" {
		data, err := yaml.Marshal(exportData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=gost-panel-export.yaml")
		c.Data(http.StatusOK, "application/x-yaml", data)
	} else {
		data, err := json.MarshalIndent(exportData, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", "attachment; filename=gost-panel-export.json")
		c.Data(http.StatusOK, "application/json", data)
	}
}

// importData 导入数据
func (s *Server) importData(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}

	// 读取文件内容
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
		return
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	// 解析数据
	var importData ExportData

	// 尝试 JSON 解析
	if err := json.Unmarshal(content, &importData); err != nil {
		// 尝试 YAML 解析
		if err := yaml.Unmarshal(content, &importData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file format (expected JSON or YAML)"})
			return
		}
	}

	// 导入统计
	var nodesCreated, nodesSkipped, clientsCreated, clientsSkipped int

	// 预先查询所有节点和客户端，避免 N+1 查询
	existingNodes, _ := s.svc.ListNodes()
	existingClients, _ := s.svc.ListClients()

	// 构建名称查找 map
	nodeNameSet := make(map[string]bool)
	nodeNameToID := make(map[string]uint)
	for _, n := range existingNodes {
		nodeNameSet[n.Name] = true
		nodeNameToID[n.Name] = n.ID
	}
	clientNameSet := make(map[string]bool)
	for _, c := range existingClients {
		clientNameSet[c.Name] = true
	}

	// 导入节点
	for _, n := range importData.Nodes {
		// 检查是否已存在同名节点
		if nodeNameSet[n.Name] {
			nodesSkipped++
			continue
		}

		node := &model.Node{
			Name:       n.Name,
			Host:       n.Host,
			Port:       n.Port,
			Protocol:   n.Protocol,
			Transport:  n.Transport,
			ProxyUser:  n.ProxyUser,
			ProxyPass:  n.ProxyPass,
			SSMethod:   n.SSMethod,
			SSPassword: n.SSPassword,
			TLSEnabled: n.TLSEnabled,
			TLSSNI:     n.TLSSNI,
			WSPath:     n.WSPath,
			WSHost:     n.WSHost,
		}
		if err := s.svc.CreateNode(node); err == nil {
			nodesCreated++
			// 更新 map 以便客户端导入时可以找到新创建的节点
			nodeNameSet[n.Name] = true
			nodeNameToID[n.Name] = node.ID
		}
	}

	// 导入客户端
	for _, cl := range importData.Clients {
		// 检查是否已存在同名客户端
		if clientNameSet[cl.Name] {
			clientsSkipped++
			continue
		}

		// 查找节点 ID
		var nodeID uint
		if cl.NodeName != "" {
			nodeID = nodeNameToID[cl.NodeName]
		}

		client := &model.Client{
			Name:       cl.Name,
			NodeID:     nodeID,
			LocalPort:  cl.LocalPort,
			RemotePort: cl.RemotePort,
			ProxyUser:  cl.ProxyUser,
			ProxyPass:  cl.ProxyPass,
		}
		if err := s.svc.CreateClient(client); err == nil {
			clientsCreated++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Import completed",
		"nodes_created":   nodesCreated,
		"nodes_skipped":   nodesSkipped,
		"clients_created": clientsCreated,
		"clients_skipped": clientsSkipped,
	})
}

// ==================== 数据库备份/恢复 ====================

// backupDatabase 下载数据库备份
func (s *Server) backupDatabase(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	dbPath := s.cfg.DBPath

	// 检查文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "database file not found"})
		return
	}

	// 创建临时备份文件（避免读写冲突）
	backupPath := dbPath + ".backup"

	// 复制数据库文件
	src, err := os.Open(dbPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open database"})
		return
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create backup"})
		return
	}
	defer dst.Close()
	defer os.Remove(backupPath) // 清理临时文件

	if _, err := io.Copy(dst, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to copy database"})
		return
	}
	dst.Close()

	// 发送备份文件
	filename := fmt.Sprintf("gost-panel-backup-%s.db", time.Now().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.File(backupPath)
}

// restoreDatabase 恢复数据库
func (s *Server) restoreDatabase(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	file, err := c.FormFile("backup")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no backup file provided"})
		return
	}

	// 检查文件大小 (最大 100MB)
	if file.Size > 100*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 100MB)"})
		return
	}

	// 保存上传的文件到临时位置
	tempPath := s.cfg.DBPath + ".restore"
	if err := c.SaveUploadedFile(file, tempPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save uploaded file"})
		return
	}
	defer os.Remove(tempPath)

	// 验证是 SQLite 数据库
	testDB, err := model.InitDB(tempPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid database file"})
		return
	}
	sqlDB, _ := testDB.DB()
	sqlDB.Close()

	// 备份当前数据库
	currentBackup := s.cfg.DBPath + ".bak"
	if err := copyFile(s.cfg.DBPath, currentBackup); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to backup current database"})
		return
	}

	// 替换数据库文件
	if err := copyFile(tempPath, s.cfg.DBPath); err != nil {
		// 恢复失败，还原备份
		copyFile(currentBackup, s.cfg.DBPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Database restored successfully. Please restart the service.",
	})
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// 确保目标目录存在
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// ==================== 代理链/隧道转发 ====================

func (s *Server) listProxyChains(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	var ownerID *uint
	if !isAdmin {
		ownerID = &userID
	}
	chains, err := s.svc.ListProxyChains(ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, chains)
}

func (s *Server) getProxyChain(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	chain, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy chain not found"})
		return
	}
	c.JSON(http.StatusOK, chain)
}

func (s *Server) createProxyChain(c *gin.Context) {
	var chain model.ProxyChain
	if err := c.ShouldBindJSON(&chain); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "proxy_chain")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}
	}

	// 强制设置所有者 (防止用户指定任意 owner_id)
	chain.OwnerID = &userID

	if err := s.svc.CreateProxyChain(&chain); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, chain)
}

func (s *Server) updateProxyChain(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 防止篡改受保护字段
	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "created_at")

	if err := s.svc.UpdateProxyChainMap(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, _ := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin)
	c.JSON(http.StatusOK, result)
}

func (s *Server) deleteProxyChain(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}

	if err := s.svc.DeleteProxyChain(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) listProxyChainHops(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}
	hops, err := s.svc.GetProxyChainHopsWithNodes(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, hops)
}

func (s *Server) addProxyChainHop(c *gin.Context) {
	chainID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(chainID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}

	var hop model.ProxyChainHop
	if err := c.ShouldBindJSON(&hop); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(hop.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	hop.ChainID = uint(chainID)

	// 获取当前最大顺序号
	hops, _ := s.svc.GetProxyChainHops(uint(chainID))
	hop.HopOrder = len(hops)

	if err := s.svc.AddProxyChainHop(&hop); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, hop)
}

func (s *Server) updateProxyChainHop(c *gin.Context) {
	chainID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(chainID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}
	hopID, _ := strconv.ParseUint(c.Param("hopId"), 10, 32)
	if err := s.svc.DB().Where("id = ? AND chain_id = ?", uint(hopID), uint(chainID)).First(&model.ProxyChainHop{}).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hop not found"})
		return
	}

	var hop model.ProxyChainHop
	if err := c.ShouldBindJSON(&hop); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(hop.NodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}

	hop.ID = uint(hopID)
	hop.ChainID = uint(chainID)
	if err := s.svc.UpdateProxyChainHop(&hop); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, hop)
}

func (s *Server) removeProxyChainHop(c *gin.Context) {
	chainID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetProxyChainByOwner(uint(chainID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此代理链"})
		return
	}
	hopID, _ := strconv.ParseUint(c.Param("hopId"), 10, 32)
	if err := s.svc.DB().Where("id = ? AND chain_id = ?", uint(hopID), uint(chainID)).First(&model.ProxyChainHop{}).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "hop not found"})
		return
	}
	if err := s.svc.RemoveProxyChainHop(uint(hopID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getProxyChainConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	chain, err := s.svc.GetProxyChainByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy chain not found"})
		return
	}

	hops, err := s.svc.GetProxyChainHopsWithNodes(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateProxyChainFullConfig(chain, hops)

	c.YAML(http.StatusOK, config)
}

// ==================== 隧道转发 (入口-出口模式) ====================

func (s *Server) listTunnels(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	var ownerID *uint
	if !isAdmin {
		ownerID = &userID
	}
	tunnels, err := s.svc.ListTunnels(ownerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tunnels)
}

func (s *Server) getTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}
	c.JSON(http.StatusOK, tunnel)
}

func (s *Server) createTunnel(c *gin.Context) {
	var tunnel model.Tunnel
	if err := c.ShouldBindJSON(&tunnel); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, isAdmin := getUserInfo(c)

	// 检查套餐资源限制
	if !isAdmin {
		allowed, msg := s.svc.CheckPlanResourceLimit(userID, "tunnel")
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": msg})
			return
		}

		// 检查入口节点访问权限
		allowed, msg = s.svc.CheckPlanNodeAccess(userID, tunnel.EntryNodeID)
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "入口" + msg})
			return
		}

		// 检查出口节点访问权限
		allowed, msg = s.svc.CheckPlanNodeAccess(userID, tunnel.ExitNodeID)
		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "出口" + msg})
			return
		}
	}

	// 强制设置所有者 (防止用户指定任意 owner_id)
	if !isAdmin {
		if _, err := s.svc.GetNodeByOwner(tunnel.EntryNodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
		if _, err := s.svc.GetNodeByOwner(tunnel.ExitNodeID, userID, isAdmin); err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
			return
		}
	}
	tunnel.OwnerID = &userID

	if err := s.svc.CreateTunnel(&tunnel); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 重新加载以获取关联数据
	result, _ := s.svc.GetTunnel(tunnel.ID)
	c.JSON(http.StatusOK, result)
}

func (s *Server) updateTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此隧道"})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 防止篡改受保护字段
	delete(updates, "id")
	delete(updates, "owner_id")
	delete(updates, "created_at")

	if !isAdmin {
		if rawEntryNodeID, exists := updates["entry_node_id"]; exists {
			if rawEntryNodeID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid entry_node_id"})
				return
			}
			entryNodeID, err := parseUintField(rawEntryNodeID, "entry_node_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			allowed, msg := s.svc.CheckPlanNodeAccess(userID, entryNodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": "鍏ュ彛" + msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(entryNodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
		if rawExitNodeID, exists := updates["exit_node_id"]; exists {
			if rawExitNodeID == nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid exit_node_id"})
				return
			}
			exitNodeID, err := parseUintField(rawExitNodeID, "exit_node_id")
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			allowed, msg := s.svc.CheckPlanNodeAccess(userID, exitNodeID)
			if !allowed {
				c.JSON(http.StatusForbidden, gin.H{"error": "鍑哄彛" + msg})
				return
			}
			if _, err := s.svc.GetNodeByOwner(exitNodeID, userID, isAdmin); err != nil {
				c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
				return
			}
		}
	}

	if err := s.svc.UpdateTunnelMap(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	result, _ := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	c.JSON(http.StatusOK, result)
}

func (s *Server) deleteTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	// 权限检查
	if _, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此隧道"})
		return
	}

	if err := s.svc.DeleteTunnel(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) syncTunnel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	// 检查入口节点是否存在
	if tunnel.EntryNode == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "entry node not found"})
		return
	}

	// 检查入口节点是否在线
	if tunnel.EntryNode.Status != "online" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "entry node is offline",
			"message": "入口节点离线，无法同步配置",
		})
		return
	}

	// 生成隧道配置
	generator := gost.NewConfigGenerator()
	config := generator.GenerateTunnelEntryConfig(tunnel)
	if config == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to generate tunnel config"})
		return
	}

	// 连接入口节点的 GOST API
	client := gost.NewClient(
		tunnel.EntryNode.Host,
		tunnel.EntryNode.APIPort,
		tunnel.EntryNode.APIUser,
		tunnel.EntryNode.APIPass,
	)

	// 同步配置
	if err := client.SyncTunnelConfig(config, tunnel.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "sync failed",
			"message": fmt.Sprintf("同步到入口节点失败: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("隧道配置已同步到入口节点 %s", tunnel.EntryNode.Name),
	})
}

func (s *Server) getTunnelEntryConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateTunnelEntryConfig(tunnel)

	c.YAML(http.StatusOK, config)
}

func (s *Server) getTunnelExitConfig(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	tunnel, err := s.svc.GetTunnelByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tunnel not found"})
		return
	}

	generator := gost.NewConfigGenerator()
	config := generator.GenerateTunnelExitConfig(tunnel)

	c.YAML(http.StatusOK, config)
}

// ==================== 网站配置 ====================

func (s *Server) getSiteConfigs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	configs := s.svc.GetSiteConfigs()
	c.JSON(http.StatusOK, configs)
}

func (s *Server) updateSiteConfigs(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var configs map[string]string
	if err := c.ShouldBindJSON(&configs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.SetSiteConfigs(configs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "配置已保存"})
}

// getPublicSiteConfig 公开接口，无需认证
func (s *Server) getPublicSiteConfig(c *gin.Context) {
	configs := s.svc.GetSiteConfigs()
	// 只返回前端需要的公开配置
	public := map[string]string{
		"site_name":        configs["site_name"],
		"site_description": configs["site_description"],
		"favicon_url":      configs["favicon_url"],
		"logo_url":         configs["logo_url"],
		"footer_text":      configs["footer_text"],
		"custom_css":       configs["custom_css"],
	}
	c.JSON(http.StatusOK, public)
}

// ==================== 节点标签管理 ====================

// listTags 获取所有标签
func (s *Server) listTags(c *gin.Context) {
	tags, err := s.svc.ListTags()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tags)
}

// getTag 获取单个标签
func (s *Server) getTag(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	tag, err := s.svc.GetTag(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}
	c.JSON(http.StatusOK, tag)
}

// CreateTagRequest 创建标签请求
type CreateTagRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color"`
}

// createTag 创建标签
func (s *Server) createTag(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可管理标签"})
		return
	}
	var req CreateTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tag := &model.Tag{
		Name:  req.Name,
		Color: req.Color,
	}
	if tag.Color == "" {
		tag.Color = "#3b82f6" // 默认蓝色
	}

	if err := s.svc.CreateTag(tag); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, tag)
}

// updateTag 更新标签
func (s *Server) updateTag(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可管理标签"})
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.UpdateTag(uint(id), updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已更新"})
}

// deleteTag 删除标签
func (s *Server) deleteTag(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可管理标签"})
		return
	}
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.DeleteTag(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "标签已删除"})
}

// getNodeTags 获取节点的标签
func (s *Server) getNodeTags(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}
	tags, err := s.svc.GetNodeTags(uint(nodeID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tags)
}

// NodeTagRequest 节点标签请求
type NodeTagRequest struct {
	TagID uint `json:"tag_id" binding:"required"`
}

// addNodeTag 给节点添加标签
func (s *Server) addNodeTag(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}

	var req NodeTagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.AddNodeTag(uint(nodeID), req.TagID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已添加"})
}

// removeNodeTag 从节点移除标签
func (s *Server) removeNodeTag(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}
	tagID, _ := strconv.ParseUint(c.Param("tagId"), 10, 32)

	if err := s.svc.RemoveNodeTag(uint(nodeID), uint(tagID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "标签已移除"})
}

// SetNodeTagsRequest 设置节点标签请求
type SetNodeTagsRequest struct {
	TagIDs []uint `json:"tag_ids"`
}

// setNodeTags 设置节点的所有标签
func (s *Server) setNodeTags(c *gin.Context) {
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(nodeID), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}

	var req SetNodeTagsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.SetNodeTags(uint(nodeID), req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "节点标签已更新"})
}

// getNodesByTag 获取具有指定标签的节点
func (s *Server) getNodesByTag(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	tagID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	nodes, err := s.svc.GetNodesByTag(uint(tagID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Filter by ownership for non-admin users
	if !isAdmin {
		filtered := make([]model.Node, 0)
		for _, n := range nodes {
			if n.OwnerID != nil && *n.OwnerID == userID {
				filtered = append(filtered, n)
			}
		}
		nodes = filtered
	}
	c.JSON(http.StatusOK, nodes)
}

// ==================== 全局搜索 ====================

// SearchResult 全局搜索结果
type SearchResult struct {
	Type string      `json:"type"` // node, client, user
	ID   uint        `json:"id"`
	Name string      `json:"name"`
	Desc string      `json:"desc"`
	Data interface{} `json:"data,omitempty"`
}

// globalSearch 全局搜索
func (s *Server) globalSearch(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, []SearchResult{})
		return
	}

	userID, isAdmin := getUserInfo(c)
	var results []SearchResult

	// 搜索节点
	nodes, _ := s.svc.ListNodesByOwner(userID, isAdmin)
	for _, node := range nodes {
		if containsIgnoreCase(node.Name, query) || containsIgnoreCase(node.Host, query) {
			results = append(results, SearchResult{
				Type: "node",
				ID:   node.ID,
				Name: node.Name,
				Desc: fmt.Sprintf("%s:%d - %s", node.Host, node.Port, node.Status),
			})
		}
		if len(results) >= 20 {
			break
		}
	}

	// 搜索客户端
	clients, _ := s.svc.ListClientsByOwner(userID, isAdmin)
	for _, client := range clients {
		if containsIgnoreCase(client.Name, query) || containsIgnoreCase(client.Token, query) {
			tokenPreview := client.Token
			if len(tokenPreview) > 8 {
				tokenPreview = tokenPreview[:8] + "..."
			}
			results = append(results, SearchResult{
				Type: "client",
				ID:   client.ID,
				Name: client.Name,
				Desc: fmt.Sprintf("Token: %s - %s", tokenPreview, client.Status),
			})
		}
		if len(results) >= 30 {
			break
		}
	}

	// 搜索用户 (仅管理员)
	if isAdmin {
		users, _ := s.svc.ListUsers()
		for _, user := range users {
			userEmail := ""
			if user.Email != nil {
				userEmail = *user.Email
			}
			if containsIgnoreCase(user.Username, query) || containsIgnoreCase(userEmail, query) {
				results = append(results, SearchResult{
					Type: "user",
					ID:   user.ID,
					Name: user.Username,
					Desc: fmt.Sprintf("%s - %s", userEmail, user.Role),
				})
			}
			if len(results) >= 40 {
				break
			}
		}
	}

	c.JSON(http.StatusOK, results)
}

// containsIgnoreCase 忽略大小写的字符串包含检查
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// ==================== 套餐管理 ====================

func (s *Server) listPlans(c *gin.Context) {
	plans, err := s.svc.ListPlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 附加每个套餐的用户数量
	result := make([]gin.H, len(plans))
	for i, plan := range plans {
		userCount, _ := s.svc.GetPlanUserCount(plan.ID)
		result[i] = gin.H{
			"id":                plan.ID,
			"name":              plan.Name,
			"description":       plan.Description,
			"traffic_quota":     plan.TrafficQuota,
			"speed_limit":       plan.SpeedLimit,
			"duration":          plan.Duration,
			"max_nodes":         plan.MaxNodes,
			"max_clients":       plan.MaxClients,
			"max_tunnels":       plan.MaxTunnels,
			"max_port_forwards": plan.MaxPortForwards,
			"max_proxy_chains":  plan.MaxProxyChains,
			"max_node_groups":   plan.MaxNodeGroups,
			"enabled":           plan.Enabled,
			"sort_order":        plan.SortOrder,
			"user_count":        userCount,
			"created_at":        plan.CreatedAt,
			"updated_at":        plan.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) getPlan(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	plan, err := s.svc.GetPlan(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "plan not found"})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (s *Server) createPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	var plan model.Plan
	if err := c.ShouldBindJSON(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if plan.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "套餐名称不能为空"})
		return
	}

	if err := s.svc.CreatePlan(&plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, plan)
}

func (s *Server) updatePlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.UpdatePlan(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deletePlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	if err := s.svc.DeletePlan(uint(id)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 用户套餐操作 ====================

func (s *Server) assignUserPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var req struct {
		PlanID uint `json:"plan_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := s.svc.AssignUserPlan(uint(userID), req.PlanID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) removeUserPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	if err := s.svc.RemoveUserPlan(uint(userID)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) renewUserPlan(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	userID, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var req struct {
		Days int `json:"days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Days <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "续期天数必须大于0"})
		return
	}

	if err := s.svc.RenewUserPlan(uint(userID), req.Days); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== 套餐资源关联 ====================

// getPlanResources 获取套餐关联的资源
func (s *Server) getPlanResources(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	resources, err := s.svc.GetPlanResources(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 组织成 map[resourceType][]resourceID 的格式
	result := make(map[string][]uint)
	result["node"] = []uint{}
	result["tunnel"] = []uint{}
	result["port_forward"] = []uint{}
	result["proxy_chain"] = []uint{}
	result["node_group"] = []uint{}

	for _, r := range resources {
		result[r.ResourceType] = append(result[r.ResourceType], r.ResourceID)
	}

	c.JSON(http.StatusOK, result)
}

// setPlanResources 设置套餐关联的资源
func (s *Server) setPlanResources(c *gin.Context) {
	_, isAdmin := getUserInfo(c)
	if !isAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin only"})
		return
	}

	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	var req map[string][]uint
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 对每个 resourceType 调用 SetPlanResources
	validTypes := []string{"node", "tunnel", "port_forward", "proxy_chain", "node_group"}
	for _, resourceType := range validTypes {
		resourceIDs, ok := req[resourceType]
		if !ok {
			resourceIDs = []uint{}
		}
		if err := s.svc.SetPlanResources(uint(id), resourceType, resourceIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Bypass 分流规则 ====================

func (s *Server) listBypasses(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	bypasses, err := s.svc.ListBypasses(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, bypasses)
}

func (s *Server) getBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	bypass, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bypass not found"})
		return
	}
	c.JSON(http.StatusOK, bypass)
}

func (s *Server) createBypass(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var bypass model.Bypass
	if err := c.ShouldBindJSON(&bypass); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bypass.OwnerID = &userID
	if err := s.svc.CreateBypass(&bypass); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "bypass", bypass.ID, bypass.Name)
	c.JSON(http.StatusOK, bypass)
}

func (s *Server) updateBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateBypass(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "bypass", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteBypass(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "bypass", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Admission 准入控制 ====================

func (s *Server) listAdmissions(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	admissions, err := s.svc.ListAdmissions(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, admissions)
}

func (s *Server) getAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	admission, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "admission not found"})
		return
	}
	c.JSON(http.StatusOK, admission)
}

func (s *Server) createAdmission(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var admission model.Admission
	if err := c.ShouldBindJSON(&admission); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	admission.OwnerID = &userID
	if err := s.svc.CreateAdmission(&admission); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "admission", admission.ID, admission.Name)
	c.JSON(http.StatusOK, admission)
}

func (s *Server) updateAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateAdmission(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "admission", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteAdmission(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "admission", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== HostMapping 主机映射 ====================

func (s *Server) listHostMappings(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	mappings, err := s.svc.ListHostMappings(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mappings)
}

func (s *Server) getHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	mapping, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "host mapping not found"})
		return
	}
	c.JSON(http.StatusOK, mapping)
}

func (s *Server) createHostMapping(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var mapping model.HostMapping
	if err := c.ShouldBindJSON(&mapping); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mapping.OwnerID = &userID
	if err := s.svc.CreateHostMapping(&mapping); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "host_mapping", mapping.ID, mapping.Name)
	c.JSON(http.StatusOK, mapping)
}

func (s *Server) updateHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateHostMapping(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "host_mapping", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteHostMapping(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "host_mapping", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Ingress 反向代理 ====================

func (s *Server) listIngresses(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	ingresses, err := s.svc.ListIngresses(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ingresses)
}

func (s *Server) getIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	ingress, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ingress not found"})
		return
	}
	c.JSON(http.StatusOK, ingress)
}

func (s *Server) createIngress(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var ingress model.Ingress
	if err := c.ShouldBindJSON(&ingress); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ingress.OwnerID = &userID
	if err := s.svc.CreateIngress(&ingress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "ingress", ingress.ID, ingress.Name)
	c.JSON(http.StatusOK, ingress)
}

func (s *Server) updateIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateIngress(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "ingress", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteIngress(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "ingress", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Recorder 流量记录 ====================

func (s *Server) listRecorders(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	recorders, err := s.svc.ListRecorders(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, recorders)
}

func (s *Server) getRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	recorder, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recorder not found"})
		return
	}
	c.JSON(http.StatusOK, recorder)
}

func (s *Server) createRecorder(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var recorder model.Recorder
	if err := c.ShouldBindJSON(&recorder); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	recorder.OwnerID = &userID
	if err := s.svc.CreateRecorder(&recorder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "recorder", recorder.ID, recorder.Name)
	c.JSON(http.StatusOK, recorder)
}

func (s *Server) updateRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateRecorder(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "recorder", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteRecorder(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "recorder", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Router 路由管理 ====================

func (s *Server) listRouters(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	routers, err := s.svc.ListRouters(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, routers)
}

func (s *Server) getRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	router, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}
	c.JSON(http.StatusOK, router)
}

func (s *Server) createRouter(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var router model.Router
	if err := c.ShouldBindJSON(&router); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	router.OwnerID = &userID
	if err := s.svc.CreateRouter(&router); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "router", router.ID, router.Name)
	c.JSON(http.StatusOK, router)
}

func (s *Server) updateRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateRouter(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "router", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteRouter(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "router", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== SD 服务发现 ====================

func (s *Server) listSDs(c *gin.Context) {
	userID, isAdmin := getUserInfo(c)
	sds, err := s.svc.ListSDs(userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sds)
}

func (s *Server) getSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	sd, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sd not found"})
		return
	}
	c.JSON(http.StatusOK, sd)
}

func (s *Server) createSD(c *gin.Context) {
	userID, _ := getUserInfo(c)
	var sd model.SD
	if err := c.ShouldBindJSON(&sd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sd.OwnerID = &userID
	if err := s.svc.CreateSD(&sd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "create", "sd", sd.ID, sd.Name)
	c.JSON(http.StatusOK, sd)
}

func (s *Server) updateSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	delete(updates, "owner_id")
	if err := s.svc.UpdateSD(uint(id), updates); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "update", "sd", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) deleteSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此资源"})
		return
	}
	if err := s.svc.DeleteSD(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	s.audit.LogSuccess(c, "delete", "sd", uint(id), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ==================== Clone Handlers for Rules ====================

func (s *Server) cloneBypass(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	bypass, err := s.svc.GetBypassByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "bypass not found"})
		return
	}

	cloned := &model.Bypass{
		Name:      bypass.Name + " (副本)",
		Whitelist: bypass.Whitelist,
		Matchers:  bypass.Matchers,
		NodeID:    bypass.NodeID,
		OwnerID:   &userID,
	}

	if err := s.svc.CreateBypass(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "bypass", cloned.ID, fmt.Sprintf("from #%d", bypass.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneAdmission(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	admission, err := s.svc.GetAdmissionByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "admission not found"})
		return
	}

	cloned := &model.Admission{
		Name:      admission.Name + " (副本)",
		Whitelist: admission.Whitelist,
		Matchers:  admission.Matchers,
		NodeID:    admission.NodeID,
		OwnerID:   &userID,
	}

	if err := s.svc.CreateAdmission(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "admission", cloned.ID, fmt.Sprintf("from #%d", admission.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneHostMapping(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	mapping, err := s.svc.GetHostMappingByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "host mapping not found"})
		return
	}

	cloned := &model.HostMapping{
		Name:     mapping.Name + " (副本)",
		Mappings: mapping.Mappings,
		NodeID:   mapping.NodeID,
		OwnerID:  &userID,
	}

	if err := s.svc.CreateHostMapping(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "host_mapping", cloned.ID, fmt.Sprintf("from #%d", mapping.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneIngress(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	ingress, err := s.svc.GetIngressByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ingress not found"})
		return
	}

	cloned := &model.Ingress{
		Name:    ingress.Name + " (副本)",
		Rules:   ingress.Rules,
		NodeID:  ingress.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateIngress(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "ingress", cloned.ID, fmt.Sprintf("from #%d", ingress.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneRecorder(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	recorder, err := s.svc.GetRecorderByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "recorder not found"})
		return
	}

	cloned := &model.Recorder{
		Name:    recorder.Name + " (副本)",
		Type:    recorder.Type,
		Config:  recorder.Config,
		NodeID:  recorder.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateRecorder(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "recorder", cloned.ID, fmt.Sprintf("from #%d", recorder.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneRouter(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	router, err := s.svc.GetRouterByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "router not found"})
		return
	}

	cloned := &model.Router{
		Name:    router.Name + " (副本)",
		Routes:  router.Routes,
		NodeID:  router.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateRouter(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "router", cloned.ID, fmt.Sprintf("from #%d", router.ID))
	c.JSON(http.StatusOK, cloned)
}

func (s *Server) cloneSD(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	sd, err := s.svc.GetSDByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "sd not found"})
		return
	}

	cloned := &model.SD{
		Name:    sd.Name + " (副本)",
		Type:    sd.Type,
		Config:  sd.Config,
		NodeID:  sd.NodeID,
		OwnerID: &userID,
	}

	if err := s.svc.CreateSD(cloned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "clone", "sd", cloned.ID, fmt.Sprintf("from #%d", sd.ID))
	c.JSON(http.StatusOK, cloned)
}

// ==================== ConfigVersion 配置版本历史 ====================

func (s *Server) getConfigVersions(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此节点"})
		return
	}

	versions, err := s.svc.GetConfigVersions(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, versions)
}

func (s *Server) createConfigVersion(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	userID, isAdmin := getUserInfo(c)

	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取节点配置
	node, err := s.svc.GetNodeByOwner(uint(id), userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 生成 YAML 配置
	generator := gost.NewConfigGenerator()
	bypasses, _ := s.svc.GetBypassesByNode(node.ID)
	admissions, _ := s.svc.GetAdmissionsByNode(node.ID)
	hostMappings, _ := s.svc.GetHostMappingsByNode(node.ID)
	ingresses, _ := s.svc.GetIngressesByNode(node.ID)
	config := generator.GenerateNodeConfigWithRules(node, bypasses, admissions, hostMappings, ingresses)

	// 将配置序列化为 YAML 字符串
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize config"})
		return
	}

	// 保存版本
	if err := s.svc.SaveConfigVersion(uint(id), string(configYAML), req.Comment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 清理旧版本（保留最新 20 个）
	s.svc.CleanupOldVersions(uint(id), 20)

	s.audit.LogSuccess(c, "create", "config_version", uint(id), req.Comment)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) getConfigVersion(c *gin.Context) {
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 32)

	version, err := s.svc.GetConfigVersion(uint(versionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}

	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(version.NodeID, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此配置版本"})
		return
	}

	c.JSON(http.StatusOK, version)
}

func (s *Server) restoreConfigVersion(c *gin.Context) {
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 32)

	version, err := s.svc.GetConfigVersion(uint(versionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}

	// 获取节点信息
	userID, isAdmin := getUserInfo(c)
	node, err := s.svc.GetNodeByOwner(version.NodeID, userID, isAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}

	// 检查节点是否在线
	if node.Status != "online" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "node is offline",
			"message": "节点离线，无法恢复配置",
		})
		return
	}

	// 解析 YAML 配置
	var config interface{}
	if err := yaml.Unmarshal([]byte(version.Config), &config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid config format"})
		return
	}

	// 获取 GOST API 客户端
	client, err := s.svc.GetGostClient(version.NodeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to connect to GOST"})
		return
	}

	// 应用配置到 GOST（使用 GOST API 重新加载配置）
	if err := client.ReloadConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to restore config: %v", err)})
		return
	}

	s.audit.LogSuccess(c, "restore", "config_version", uint(versionID), fmt.Sprintf("restored to node #%d", version.NodeID))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "配置已恢复",
	})
}

func (s *Server) deleteConfigVersion(c *gin.Context) {
	versionID, _ := strconv.ParseUint(c.Param("versionId"), 10, 32)

	version, err := s.svc.GetConfigVersion(uint(versionID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}
	userID, isAdmin := getUserInfo(c)
	if _, err := s.svc.GetNodeByOwner(version.NodeID, userID, isAdmin); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此配置版本"})
		return
	}

	if err := s.svc.DeleteConfigVersion(uint(versionID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	s.audit.LogSuccess(c, "delete", "config_version", uint(versionID), "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
