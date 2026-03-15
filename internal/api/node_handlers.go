// 节点管理模块
package api

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/supernaga/gost-panel/internal/gost"
	"github.com/supernaga/gost-panel/internal/model"
	"github.com/supernaga/gost-panel/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-yaml"
)



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

	// 使用白名单过滤允许更新的字段
	allowedFields := map[string]bool{
		"name": true, "host": true, "port": true, "api_port": true,
		"api_user": true, "api_pass": true, "proxy_user": true, "proxy_pass": true,
		"traffic_quota": true, "quota_reset_day": true,
		"protocol": true, "transport": true, "transport_opts": true,
		"ss_method": true, "ss_password": true,
		"tls_enabled": true, "tls_cert_file": true, "tls_key_file": true,
		"tls_sni": true, "tls_alpn": true,
		"ws_path": true, "ws_host": true,
		"speed_limit": true, "conn_rate_limit": true, "dns_server": true,
		"proxy_protocol": true, "probe_resist": true, "probe_resist_value": true,
		"plugin_config": true,
	}
	filtered := make(map[string]interface{})
	for k, v := range updates {
		if allowedFields[k] {
			filtered[k] = v
		}
	}

	// 密码字段为空时不更新，防止编辑时误覆盖已有密码
	for _, key := range []string{"api_pass", "proxy_pass", "ss_password"} {
		if v, ok := filtered[key]; ok && v == "" {
			delete(filtered, key)
		}
	}

	if len(filtered) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid fields to update"})
		return
	}

	if err := s.svc.UpdateNode(id, filtered); err != nil {
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

